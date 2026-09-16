package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/archive"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

const (
	archivePageSize      = 200
	archiveRetryCount    = 5
	archiveStateInterval = 20

	// Direct and group message channels belong to no team. The sync files
	// them under these names instead.
	archiveDirectTeamName = "direct"
	archiveGroupTeamName  = "group"
)

func init() {
	archiveCommand := &cobra.Command{
		Use:   "archive",
		Short: "Archive channels to a local directory and search them offline",
		Long: "Keep a local copy of the posts and attachments you can read, including direct " +
			"and group messages, and search it without going back to the server.\n\n" +
			"A sync is incremental: the first run of a channel reads it in full, later runs " +
			"ask only for posts newer than the last one archived, so re-running is cheap and safe.",
	}

	syncCommand := &cobra.Command{
		Use:   "sync <directory>",
		Short: "Fetch new posts and attachments into the archive",
		Args:  cobra.ExactArgs(1),
		RunE:  archiveSyncRun,
	}
	syncCommand.Flags().String("channels", "mine", "Which channels to read: mine, public, or all")
	syncCommand.Flags().String("files", "none", "Which attachments to download: none, mine, or all")
	syncCommand.Flags().Float64("max-file-mb", 50, "Skip attachments larger than this many megabytes")
	syncCommand.Flags().Bool("skip-posts", false, "Go straight to attachments, using the posts already archived")
	syncCommand.Flags().String("only", "", "Only channels whose name contains this substring")
	syncCommand.Flags().Bool("full", false, "Ignore the high-water marks and re-read every channel from the start")
	syncCommand.Flags().String("since", "", "Re-read every channel back to this date (YYYY-MM-DD) and merge, to fill gaps an earlier sync left")

	searchCommand := &cobra.Command{
		Use:   "search <directory> <query>",
		Short: "Search the archived posts",
		Args:  cobra.ExactArgs(2),
		RunE:  archiveSearchRun,
	}
	searchCommand.Flags().StringP("channel", "c", "", "Only channels whose name contains this substring")
	// Shadows the root --team override on purpose: an offline search has no
	// active team to override, and here the flag narrows the archive instead.
	searchCommand.Flags().StringP("team", "T", "", "Only teams whose name contains this substring")
	searchCommand.Flags().StringP("user", "u", "", "Only posts written by this username")
	searchCommand.Flags().String("since", "", "Only posts on or after this date (YYYY-MM-DD)")
	searchCommand.Flags().String("until", "", "Only posts before or on this date (YYYY-MM-DD)")
	searchCommand.Flags().IntP("limit", "n", 50, "Maximum matches to print, newest first (0 for no limit)")
	searchCommand.Flags().Bool("regex", false, "Treat the query as a regular expression")
	searchCommand.Flags().Bool("case-sensitive", false, "Match case exactly")
	searchCommand.Flags().Bool("oldest-first", false, "Print the oldest matches first")

	statusCommand := &cobra.Command{
		Use:   "status <directory>",
		Short: "Show what the archive holds",
		Args:  cobra.ExactArgs(1),
		RunE:  archiveStatusRun,
	}

	archiveCommand.AddCommand(syncCommand, searchCommand, statusCommand)
	rootCommand.AddCommand(archiveCommand)
}

// rawPostList is the shape of /channels/{id}/posts. The posts themselves stay
// as raw JSON so the archive keeps exactly what the server sent.
type rawPostList struct {
	Order []string                   `json:"order"`
	Posts map[string]json.RawMessage `json:"posts"`
}

// postHeader is the handful of fields the archiver itself needs to read.
type postHeader struct {
	ID       string   `json:"id"`
	CreateAt int64    `json:"create_at"`
	UserID   string   `json:"user_id"`
	FileIDs  []string `json:"file_ids"`
}

// archivedPost pairs a post with the fields the archiver reads off it. One pass
// parses the batch, so a sort comparison need not parse it again.
type archivedPost struct {
	header *postHeader
	raw    json.RawMessage
}

type archiveChannel struct {
	ID          string
	Name        string
	TeamName    string
	CreateAt    int64
	DeleteAt    int64
	RawChannel  json.RawMessage
	ChannelType model.ChannelType
}

func archiveSyncRun(command *cobra.Command, arguments []string) error {
	store, err := archive.Create(arguments[0])
	if err != nil {
		return err
	}
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	channelScope, _ := command.Flags().GetString("channels")
	fileScope, _ := command.Flags().GetString("files")
	maximumFileMegabytes, _ := command.Flags().GetFloat64("max-file-mb")
	shouldSkipPosts, _ := command.Flags().GetBool("skip-posts")
	onlySubstring, _ := command.Flags().GetString("only")
	isFullSync, _ := command.Flags().GetBool("full")
	repairSinceText, _ := command.Flags().GetString("since")
	repairSince, err := parseSearchDate(repairSinceText, false)
	if err != nil {
		return err
	}

	if channelScope != "mine" && channelScope != "public" && channelScope != "all" {
		return fmt.Errorf("commands: --channels must be mine, public, or all")
	}
	if fileScope != "none" && fileScope != "mine" && fileScope != "all" {
		return fmt.Errorf("commands: --files must be none, mine, or all")
	}

	me, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return fmt.Errorf("commands: reading the current user: %w", err)
	}
	if err := store.SaveMe(me); err != nil {
		return err
	}

	// Users come first: a direct or group message channel is named after the
	// people in it, and the listing needs their usernames.
	usernames, err := archiveSyncUsers(ctx, apiClient, store)
	if err != nil {
		return err
	}
	state, err := store.LoadState()
	if err != nil {
		return err
	}

	channels, err := archiveListChannels(ctx, apiClient, me.Id, channelScope, onlySubstring, usernames, state)
	if err != nil {
		return err
	}
	rawChannels := make([]json.RawMessage, 0, len(channels))
	for _, channel := range channels {
		rawChannels = append(rawChannels, channel.RawChannel)
	}
	if err := store.MergeChannels(rawChannels); err != nil {
		return err
	}
	printer.PrintInfo("%d channels to consider", len(channels))

	if !shouldSkipPosts {
		if err := archiveSyncPosts(ctx, apiClient, store, channels, state, isFullSync, repairSince); err != nil {
			return err
		}
	}

	if fileScope != "none" {
		if err := archiveSyncFiles(ctx, apiClient, store, me, fileScope, maximumFileMegabytes); err != nil {
			return err
		}
	}

	printer.PrintSuccess("Archive up to date in %s", store.Directory())
	return nil
}

// archiveListChannels collects the channels to read. Channels archived while
// the user was a member are only returned with include_deleted, and public
// channels the user has left can still be read without joining them, which is
// the only way to recover what was written there.
//
// Direct and group message channels belong to no team. The server lists them
// with every team, so they are taken from the first listing that carries them
// and filed under the pseudo-teams "direct" and "group", named after the
// people in them. A channel already in state.json keeps the team and name it
// was archived under, so a rename on the server, or a username change, does
// not start a second file beside the first.
func archiveListChannels(ctx context.Context, apiClient *model.Client4, userId, channelScope, onlySubstring string, usernames map[string]string, state map[string]*archive.ChannelState) ([]*archiveChannel, error) {
	teams, _, err := apiClient.GetTeamsForUser(ctx, userId, "")
	if err != nil {
		return nil, fmt.Errorf("commands: listing teams: %w", err)
	}

	// Every name an earlier sync recorded is spoken for, whether or not this
	// run lists that channel again.
	claimedBy := make(map[string]string, len(state))
	for channelId, channelState := range state {
		claimedBy[archivePathKey(channelState.TeamName, channelState.ChannelName)] = channelId
	}

	var channels []*archiveChannel
	seen := map[string]struct{}{}
	for _, team := range teams {
		var paths []string
		if channelScope == "mine" || channelScope == "all" {
			paths = append(paths, fmt.Sprintf("/users/me/teams/%s/channels?include_deleted=true", team.Id))
		}
		if channelScope == "public" || channelScope == "all" {
			paths = append(paths,
				fmt.Sprintf("/teams/%s/channels", team.Id),
				fmt.Sprintf("/teams/%s/channels/deleted", team.Id))
		}
		for _, path := range paths {
			pages, err := archiveGetPages(ctx, apiClient, path, strings.Contains(path, "/users/me/"))
			if err != nil {
				return nil, err
			}
			for _, raw := range pages {
				header := struct {
					ID          string            `json:"id"`
					Name        string            `json:"name"`
					DisplayName string            `json:"display_name"`
					ChannelType model.ChannelType `json:"type"`
					CreateAt    int64             `json:"create_at"`
					DeleteAt    int64             `json:"delete_at"`
				}{}
				if err := json.Unmarshal(raw, &header); err != nil {
					return nil, fmt.Errorf("commands: parsing channel: %w", err)
				}
				if _, isSeen := seen[header.ID]; isSeen {
					continue
				}
				channel := &archiveChannel{
					ID:          header.ID,
					Name:        header.Name,
					TeamName:    team.Name,
					CreateAt:    header.CreateAt,
					DeleteAt:    header.DeleteAt,
					ChannelType: header.ChannelType,
				}
				switch header.ChannelType {
				case model.ChannelTypeOpen, model.ChannelTypePrivate:
				case model.ChannelTypeDirect:
					channel.TeamName = archiveDirectTeamName
					channel.Name = archiveDirectChannelName(header.Name, userId, usernames)
				case model.ChannelTypeGroup:
					channel.TeamName = archiveGroupTeamName
					channel.Name = header.DisplayName
					if channel.Name == "" {
						channel.Name = header.Name
					}
				default:
					continue
				}
				if previous, isKnown := state[header.ID]; isKnown && previous.TeamName != "" && previous.ChannelName != "" {
					channel.TeamName = previous.TeamName
					channel.Name = previous.ChannelName
				} else if holder, isClaimed := claimedBy[archivePathKey(channel.TeamName, channel.Name)]; isClaimed && holder != channel.ID {
					channel.Name = archiveDisambiguateName(channel.Name, channel.ID)
				}
				claimedBy[archivePathKey(channel.TeamName, channel.Name)] = channel.ID
				if onlySubstring != "" && !strings.Contains(channel.Name, onlySubstring) {
					continue
				}
				seen[header.ID] = struct{}{}
				withTeam, err := archiveAddTeamName(raw, channel.TeamName)
				if err != nil {
					return nil, err
				}
				channel.RawChannel = withTeam
				channels = append(channels, channel)
			}
		}
	}
	sort.Slice(channels, func(first, second int) bool {
		return channels[first].CreateAt < channels[second].CreateAt
	})
	return channels, nil
}

// archivePathKey is the file one team and channel name write to, which is
// what two channels must not share.
func archivePathKey(teamName, channelName string) string {
	return archive.SafeName(teamName) + "/" + archive.SafeName(channelName)
}

// archiveDisambiguateName gives a channel a name of its own by ending it with
// part of its id. The name is cut first, so the suffix is not itself lost to
// the length cap that SafeName applies.
func archiveDisambiguateName(channelName, channelId string) string {
	const suffixLength = 8
	suffix := channelId
	if len(suffix) > suffixLength {
		suffix = suffix[:suffixLength]
	}
	safe := archive.SafeName(channelName)
	if longest := archive.MaximumNameLength - len(suffix) - 1; len(safe) > longest {
		safe = safe[:longest]
	}
	return safe + "-" + suffix
}

// archiveDirectChannelName names a direct message channel after the other
// person in it, or after the user for a message to oneself. The server names
// the channel by the two user ids joined with "__".
func archiveDirectChannelName(channelName, userId string, usernames map[string]string) string {
	partnerId := userId
	for _, part := range strings.Split(channelName, "__") {
		if part != "" && part != userId {
			partnerId = part
			break
		}
	}
	if username, isKnown := usernames[partnerId]; isKnown {
		return username
	}
	return partnerId
}

// archiveAddTeamName records which team a channel belongs to, which the channel
// object itself does not say once it is out of its listing.
func archiveAddTeamName(raw json.RawMessage, teamName string) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("commands: parsing channel: %w", err)
	}
	encoded, err := json.Marshal(teamName)
	if err != nil {
		return nil, fmt.Errorf("commands: encoding team name: %w", err)
	}
	fields["_team"] = encoded
	merged, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("commands: encoding channel: %w", err)
	}
	return merged, nil
}

func archiveSyncPosts(ctx context.Context, apiClient *model.Client4, store *archive.Store, channels []*archiveChannel, state map[string]*archive.ChannelState, isFullSync bool, repairSince int64) error {
	newPostCount, unreadableCount := 0, 0
	for index, channel := range channels {
		since := int64(0)
		if previous, isKnown := state[channel.ID]; isKnown {
			since = previous.LastCreateAt
		}

		// Where to read back to. A full sync ignores the mark; a repair reads
		// back past it; a channel with no mark is read from the beginning
		// whatever the flags say, since anything less would leave it with a
		// mark that claims a history it never fetched.
		readFrom := since
		switch {
		case isFullSync:
			readFrom = 0
		case repairSince > 0 && since > 0 && repairSince < since:
			readFrom = repairSince
		}

		collected, err := archiveReadChannel(ctx, apiClient, channel.ID, readFrom)
		if err != nil {
			printer.PrintInfo("  cannot read %s/%s: %v", channel.TeamName, channel.Name, err)
			unreadableCount++
			continue
		}

		lastCreateAt := since
		if len(collected) > 0 {
			written, err := archiveWritePosts(store, channel, collected, readFrom, readFrom < since || since == 0)
			if err != nil {
				return err
			}
			newPostCount += len(written.fresh)
			// The mark follows the newest post the server showed, new to the
			// archive or not. A run cut short re-reads posts it already has,
			// and a mark that only ever followed new ones would sit behind
			// that channel's file and page it from there on every later sync.
			if written.newestCreateAt > lastCreateAt {
				lastCreateAt = written.newestCreateAt
			}
		}

		state[channel.ID] = &archive.ChannelState{
			TeamName:     channel.TeamName,
			ChannelName:  channel.Name,
			LastCreateAt: lastCreateAt,
			IsArchived:   channel.DeleteAt != 0,
		}
		// The mark is saved as soon as the posts it covers are on disk. A run
		// cut short between the two would otherwise read those posts again
		// and, on the ordinary append path, store them twice.
		if lastCreateAt != since {
			if err := store.SaveState(state); err != nil {
				return err
			}
		}
		if (index+1)%archiveStateInterval == 0 {
			printer.PrintInfo("  %d/%d channels, %d new posts", index+1, len(channels), newPostCount)
		}
	}

	if err := store.SaveState(state); err != nil {
		return err
	}
	printer.PrintInfo("%d new posts, %d channels unreadable", newPostCount, unreadableCount)
	return nil
}

// archiveReadChannel returns the posts of one channel newer than the mark,
// keyed by post id so a post returned twice is stored once.
//
// It asks two different questions of the server, because neither endpoint
// answers both well. The since endpoint says cheaply whether a channel has
// anything new, but it cannot enumerate: its response is capped at about a
// thousand posts and ordered by when a post was last updated rather than when
// it was written, so the newest post it returns can sit far ahead of posts it
// never carried, and a mark walked forward by that value steps over them for
// good. Paging enumerates completely, running newest first in creation order,
// so reading until a page reaches the mark misses nothing, but it costs a page
// of posts for every channel whether or not anything happened in it.
//
// So: ask since whether there is anything to do, and page when there is. A
// post written after the mark always has an update time after the mark too, so
// since never says no when the answer is yes.
//
// Two things this does not promise. It holds one channel's read in memory, so
// a first read of a very long channel costs about what that channel's file
// costs on disk. And paging is by offset over a live channel, so a post
// written while the walk is in progress can shift the window and be missed;
// the next sync does not go back for it, and --since is what recovers it.
//
// Deleted posts are the one thing this does not archive. Paging omits them and
// include_deleted needs system admin, so a post deleted after it was written is
// kept only if a sync saw it while it was still there.
func archiveReadChannel(ctx context.Context, apiClient *model.Client4, channelId string, since int64) (map[string]json.RawMessage, error) {
	if since > 0 {
		hasNew, err := archiveChannelHasNewPosts(ctx, apiClient, channelId, since)
		if err != nil {
			return nil, err
		}
		if !hasNew {
			return nil, nil
		}
	}

	collected := map[string]json.RawMessage{}
	for page := 0; ; page++ {
		list, err := archiveGetPosts(ctx, apiClient,
			fmt.Sprintf("/channels/%s/posts?page=%d&per_page=%d", channelId, page, archivePageSize))
		if err != nil {
			return nil, err
		}
		for postId, raw := range list.Posts {
			collected[postId] = raw
		}

		if len(list.Order) < archivePageSize {
			return collected, nil // the start of the channel
		}

		// How far back this page reached is read off its own order. The posts
		// map also carries the thread root of any reply on the page, and one of
		// those can be years older than anything the page itself holds.
		oldestInPage := int64(0)
		for _, postId := range list.Order {
			raw, isPresent := list.Posts[postId]
			if !isPresent {
				continue
			}
			header := &postHeader{}
			if err := json.Unmarshal(raw, header); err != nil {
				return nil, fmt.Errorf("commands: parsing post: %w", err)
			}
			if oldestInPage == 0 || header.CreateAt < oldestInPage {
				oldestInPage = header.CreateAt
			}
		}
		if since > 0 && oldestInPage <= since {
			return collected, nil // back as far as what is already archived
		}
	}
}

// archiveChannelHasNewPosts reports whether anything was written in a channel
// after the mark. One request, and for a quiet channel it is the only one.
//
// Strictly after: the post the mark was taken from sits at the mark itself,
// and asking about it would page every channel on every sync. A post sharing
// that millisecond therefore waits for a later post to bring the sync back,
// and the ids kept in the state are what stop it being skipped then.
func archiveChannelHasNewPosts(ctx context.Context, apiClient *model.Client4, channelId string, since int64) (bool, error) {
	list, err := archiveGetPosts(ctx, apiClient,
		fmt.Sprintf("/channels/%s/posts?since=%d", channelId, since))
	if err != nil {
		return false, err
	}
	for _, raw := range list.Posts {
		header := &postHeader{}
		if err := json.Unmarshal(raw, header); err != nil {
			return false, fmt.Errorf("commands: parsing post: %w", err)
		}
		if header.CreateAt > since {
			return true, nil
		}
	}
	return false, nil
}

// archiveWritePosts adds the posts a read collected to one channel's file and
// records their attachments, returning the posts that were actually new.
//
// The ordinary read starts at the mark, and everything it brings back is newer
// than anything on disk, so it is appended without looking at the file: the
// largest channel files run to hundreds of megabytes, and reading one on every
// sync is what an incremental sync exists to avoid. A read that reached behind
// the mark, for --full, --since or a channel with no mark, consults the file
// by post id, so a post is stored once however it was reached, and posts the
// server no longer hands out, such as deleted ones an earlier sync caught
// while they were still there, are kept: the file is merged into, never
// replaced from the server's view.
func archiveWritePosts(store *archive.Store, channel *archiveChannel, collected map[string]json.RawMessage, readFrom int64, shouldConsultDisk bool) (*archiveWriteResult, error) {
	alreadyArchived := map[string]struct{}{}
	newestOnDisk := int64(0)
	if !shouldConsultDisk {
		// The mark says nothing on disk is newer than readFrom, and the end
		// of the file is cheap to check against that. It disagrees when a run
		// was cut short after writing posts but before saving the mark.
		//
		// The posts at the very end also have to be named. A read starts at
		// the mark rather than past it, so that a post sharing the mark's
		// millisecond is not skipped for good, and their ids are the only
		// thing telling those two apart.
		tail, err := store.ReadTail(channel.TeamName, channel.Name)
		if err != nil {
			return nil, err
		}
		switch {
		case tail.NewestCreateAt > readFrom || !tail.IsComplete:
			shouldConsultDisk = true
		default:
			newestOnDisk = tail.NewestCreateAt
			for _, raw := range tail.Posts {
				header := &postHeader{}
				if err := json.Unmarshal(raw, header); err != nil {
					shouldConsultDisk = true
					break
				}
				alreadyArchived[header.ID] = struct{}{}
			}
		}
	}
	if shouldConsultDisk {
		alreadyArchived = map[string]struct{}{}
		newestOnDisk = 0
		unparseableCount := 0
		err := store.ScanPosts(channel.TeamName, channel.Name, func(line []byte) bool {
			header := &postHeader{}
			if json.Unmarshal(line, header) != nil {
				unparseableCount++
				return true
			}
			alreadyArchived[header.ID] = struct{}{}
			if header.CreateAt > newestOnDisk {
				newestOnDisk = header.CreateAt
			}
			return true
		})
		if err != nil {
			return nil, err
		}
		if unparseableCount > 0 {
			log.Warningf("%d lines in the archive of %s/%s do not parse and were kept as they are", unparseableCount, channel.TeamName, channel.Name)
		}
	}

	result := &archiveWriteResult{}
	var fresh []*archivedPost
	oldestFresh := int64(0)
	for _, raw := range collected {
		header := &postHeader{}
		if err := json.Unmarshal(raw, header); err != nil {
			return nil, fmt.Errorf("commands: parsing post: %w", err)
		}
		// Everything collected is either already archived or about to be, so
		// the newest of them is where the mark belongs.
		if header.CreateAt > result.newestCreateAt {
			result.newestCreateAt = header.CreateAt
		}
		// At the mark rather than past it: a post written in the same
		// millisecond as the last one archived is only told apart by its id.
		if header.CreateAt < readFrom {
			continue
		}
		if _, isKnown := alreadyArchived[header.ID]; isKnown {
			continue
		}
		fresh = append(fresh, &archivedPost{header: header, raw: raw})
		if oldestFresh == 0 || header.CreateAt < oldestFresh {
			oldestFresh = header.CreateAt
		}
	}
	if len(fresh) == 0 {
		return result, nil
	}
	result.fresh = fresh
	sort.SliceStable(fresh, func(first, second int) bool {
		return fresh[first].header.CreateAt < fresh[second].header.CreateAt
	})

	lines := make([]json.RawMessage, 0, len(fresh))
	var records []*archive.ArchivedFile
	for _, post := range fresh {
		lines = append(lines, post.raw)
		for _, fileId := range post.header.FileIDs {
			records = append(records, &archive.ArchivedFile{
				FileID:      fileId,
				PostID:      post.header.ID,
				ChannelName: channel.Name,
				TeamName:    channel.TeamName,
				CreateAt:    post.header.CreateAt,
			})
		}
	}

	// The attachment index is written first. A crash between the two writes
	// then leaves a record for a post that is not archived yet, which the
	// next run simply writes again, rather than an archived post whose
	// attachments nothing will ever ask for: a later run skips that post as
	// already archived and would never regenerate its records.
	if err := store.AppendFiles(records); err != nil {
		return nil, err
	}
	// New posts that all sit after what is on disk are appended. Anything
	// older goes through a rewrite, so the file stays oldest first.
	if oldestFresh >= newestOnDisk {
		if err := store.AppendPosts(channel.TeamName, channel.Name, lines); err != nil {
			return nil, err
		}
	} else if err := store.MergePosts(channel.TeamName, channel.Name, lines); err != nil {
		return nil, err
	}
	return result, nil
}

// archiveWriteResult says what one channel's write did. It carries the posts
// that were new to the archive, and the newest post the server showed, which
// is where the next sync's mark belongs.
type archiveWriteResult struct {
	fresh          []*archivedPost
	newestCreateAt int64
}

func archiveGetPosts(ctx context.Context, apiClient *model.Client4, path string) (*rawPostList, error) {
	body, err := archiveGet(ctx, apiClient, path)
	if err != nil {
		return nil, err
	}
	list := &rawPostList{}
	if err := json.Unmarshal(body, list); err != nil {
		return nil, fmt.Errorf("commands: parsing posts: %w", err)
	}
	return list, nil
}

// archiveSyncUsers records every user on the server and returns their
// usernames by id.
func archiveSyncUsers(ctx context.Context, apiClient *model.Client4, store *archive.Store) (map[string]string, error) {
	var users []*model.User
	for page := 0; ; page++ {
		batch, _, err := apiClient.GetUsers(ctx, page, archivePageSize, "")
		if err != nil {
			return nil, fmt.Errorf("commands: listing users: %w", err)
		}
		users = append(users, batch...)
		if len(batch) < archivePageSize {
			break
		}
	}
	if err := store.SaveUsers(users); err != nil {
		return nil, err
	}
	printer.PrintInfo("%d users recorded", len(users))
	usernames := make(map[string]string, len(users))
	for _, user := range users {
		usernames[user.Id] = user.Username
	}
	return usernames, nil
}

func archiveSyncFiles(ctx context.Context, apiClient *model.Client4, store *archive.Store, me *model.User, fileScope string, maximumFileMegabytes float64) error {
	wanted, err := store.ReferencedFileIDs()
	if err != nil {
		return err
	}
	if fileScope == "mine" {
		mine, err := archiveOwnFileIds(store, me.Id)
		if err != nil {
			return err
		}
		for fileId := range wanted {
			if _, isMine := mine[fileId]; !isMine {
				delete(wanted, fileId)
			}
		}
	}

	filesDirectory := store.FilesDirectory()
	if err := os.MkdirAll(filesDirectory, 0o755); err != nil {
		return fmt.Errorf("commands: creating %s: %w", filesDirectory, err)
	}
	// One listing rather than a stat per file: the directory grows as we go.
	entries, err := os.ReadDir(filesDirectory)
	if err != nil {
		return fmt.Errorf("commands: reading %s: %w", filesDirectory, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if index := strings.Index(name, "__"); index > 0 {
			delete(wanted, name[:index])
		}
	}

	todo := make([]string, 0, len(wanted))
	for fileId := range wanted {
		todo = append(todo, fileId)
	}
	sort.Strings(todo)
	printer.PrintInfo("%d attachments to fetch in scope %q", len(todo), fileScope)

	maximumBytes := int64(maximumFileMegabytes * 1024 * 1024)
	savedCount, tooBigCount, unavailableCount := 0, 0, 0
	var savedBytes int64
	for index, fileId := range todo {
		info, _, err := apiClient.GetFileInfo(ctx, fileId)
		if err != nil {
			log.Warningf("attachment %s unavailable: %v", fileId, err)
			unavailableCount++
			continue
		}
		if info.Size > maximumBytes {
			tooBigCount++
			continue
		}
		content, _, err := apiClient.DownloadFile(ctx, fileId, true)
		if err != nil {
			log.Warningf("attachment %s (%s) unavailable: %v", fileId, info.Name, err)
			unavailableCount++
			continue
		}
		path := filepath.Join(filesDirectory, fileId+"__"+archive.SafeName(info.Name))
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return fmt.Errorf("commands: writing %s: %w", path, err)
		}
		savedCount++
		savedBytes += int64(len(content))
		if (index+1)%200 == 0 {
			printer.PrintInfo("  %d/%d attachments, %d saved, %.2f GB",
				index+1, len(todo), savedCount, float64(savedBytes)/1e9)
		}
	}
	printer.PrintInfo("%d attachments saved (%.2f GB), %d over %.0f MB, %d unavailable",
		savedCount, float64(savedBytes)/1e9, tooBigCount, maximumFileMegabytes, unavailableCount)
	return nil
}

// archiveOwnFileIds reads the attachment ids on the user's own posts out of the
// archive, so "only my files" needs no further server calls.
func archiveOwnFileIds(store *archive.Store, userId string) (map[string]struct{}, error) {
	fileIds := map[string]struct{}{}
	channelFiles, err := store.ChannelFiles()
	if err != nil {
		return nil, err
	}
	for _, channelFile := range channelFiles {
		err := archive.ReadPosts(channelFile.Path, func(line []byte) bool {
			header := &postHeader{}
			if json.Unmarshal(line, header) != nil || header.UserID != userId {
				return true
			}
			for _, fileId := range header.FileIDs {
				fileIds[fileId] = struct{}{}
			}
			return true
		})
		if err != nil {
			return nil, err
		}
	}
	return fileIds, nil
}

func archiveStatusRun(command *cobra.Command, arguments []string) error {
	store, err := archive.Open(arguments[0])
	if err != nil {
		return err
	}
	state, err := store.LoadState()
	if err != nil {
		return err
	}
	channelFiles, err := store.ChannelFiles()
	if err != nil {
		return err
	}

	postCount := int64(0)
	var latest int64
	for _, channelFile := range channelFiles {
		err := archive.ReadPosts(channelFile.Path, func(line []byte) bool {
			postCount++
			return true
		})
		if err != nil {
			return err
		}
	}
	for _, channelState := range state {
		if channelState.LastCreateAt > latest {
			latest = channelState.LastCreateAt
		}
	}

	if printer.JSONOutput {
		printer.PrintJSON(map[string]interface{}{
			"directory":    store.Directory(),
			"channels":     len(channelFiles),
			"posts":        postCount,
			"last_post_at": latest,
		})
		return nil
	}
	printer.PrintInfo("Directory:  %s", store.Directory())
	printer.PrintInfo("Channels:   %d", len(channelFiles))
	printer.PrintInfo("Posts:      %d", postCount)
	if latest > 0 {
		printer.PrintInfo("Newest:     %s", time.UnixMilli(latest).Format("2006-01-02 15:04"))
	}
	return nil
}

// archiveGetPages reads a listing, paging it unless the endpoint returns
// everything at once.
func archiveGetPages(ctx context.Context, apiClient *model.Client4, path string, isSinglePage bool) ([]json.RawMessage, error) {
	if isSinglePage {
		body, err := archiveGet(ctx, apiClient, path)
		if err != nil {
			return nil, err
		}
		var batch []json.RawMessage
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("commands: parsing %s: %w", path, err)
		}
		return batch, nil
	}

	var collected []json.RawMessage
	for page := 0; ; page++ {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		body, err := archiveGet(ctx, apiClient,
			fmt.Sprintf("%s%spage=%d&per_page=%d", path, separator, page, archivePageSize))
		if err != nil {
			return nil, err
		}
		var batch []json.RawMessage
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("commands: parsing %s: %w", path, err)
		}
		collected = append(collected, batch...)
		if len(batch) < archivePageSize {
			return collected, nil
		}
	}
}

// archiveGet reads one API path as raw JSON, retrying the failures that are
// worth retrying. A 4xx other than 408 or 429 is the server saying no, and
// asking again with the same request and the same token will not change its
// mind, so it is returned as is.
func archiveGet(ctx context.Context, apiClient *model.Client4, path string) ([]byte, error) {
	var lastError error
	for attempt := 0; attempt < archiveRetryCount; attempt++ {
		response, err := apiClient.DoAPIGet(ctx, path, "")
		if err == nil {
			body, readError := readBody(response)
			if readError == nil {
				return body, nil
			}
			lastError = readError
		} else {
			if response != nil && response.StatusCode >= http.StatusBadRequest &&
				response.StatusCode < http.StatusInternalServerError &&
				response.StatusCode != http.StatusTooManyRequests && response.StatusCode != http.StatusRequestTimeout {
				return nil, fmt.Errorf("commands: reading %s: %w", path, err)
			}
			lastError = err
		}
		if attempt < archiveRetryCount-1 {
			time.Sleep(time.Duration(2*(attempt+1)) * time.Second)
		}
	}
	return nil, fmt.Errorf("commands: reading %s: %w", path, lastError)
}

func readBody(response *http.Response) ([]byte, error) {
	defer func() { _ = response.Body.Close() }()
	return io.ReadAll(response.Body)
}
