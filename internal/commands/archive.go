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
)

func init() {
	archiveCommand := &cobra.Command{
		Use:   "archive",
		Short: "Archive channels to a local directory and search them offline",
		Long: "Keep a local copy of the posts and attachments you can read, and search it " +
			"without going back to the server.\n\n" +
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
	syncCommand.Flags().Float64("max-file-mb", 25, "Skip attachments larger than this many megabytes")
	syncCommand.Flags().Bool("skip-posts", false, "Go straight to attachments, using the posts already archived")
	syncCommand.Flags().String("only", "", "Only channels whose name contains this substring")
	syncCommand.Flags().Bool("full", false, "Ignore the high-water marks and re-read every channel from the start")

	searchCommand := &cobra.Command{
		Use:   "search <directory> <query>",
		Short: "Search the archived posts",
		Args:  cobra.ExactArgs(2),
		RunE:  archiveSearchRun,
	}
	searchCommand.Flags().StringP("channel", "c", "", "Only channels whose name contains this substring")
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
	store, err := archive.Open(arguments[0])
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

	channels, err := archiveListChannels(ctx, apiClient, me.Id, channelScope, onlySubstring)
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
		if err := archiveSyncPosts(ctx, apiClient, store, channels, isFullSync); err != nil {
			return err
		}
	}

	if err := archiveSyncUsers(ctx, apiClient, store); err != nil {
		return err
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
func archiveListChannels(ctx context.Context, apiClient *model.Client4, userId, channelScope, onlySubstring string) ([]*archiveChannel, error) {
	teams, _, err := apiClient.GetTeamsForUser(ctx, userId, "")
	if err != nil {
		return nil, fmt.Errorf("commands: listing teams: %w", err)
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
				channel := &archiveChannel{}
				header := struct {
					ID          string            `json:"id"`
					Name        string            `json:"name"`
					ChannelType model.ChannelType `json:"type"`
					CreateAt    int64             `json:"create_at"`
					DeleteAt    int64             `json:"delete_at"`
				}{}
				if err := json.Unmarshal(raw, &header); err != nil {
					return nil, fmt.Errorf("commands: parsing channel: %w", err)
				}
				if header.ChannelType != model.ChannelTypeOpen && header.ChannelType != model.ChannelTypePrivate {
					continue
				}
				if _, isSeen := seen[header.ID]; isSeen {
					continue
				}
				if onlySubstring != "" && !strings.Contains(header.Name, onlySubstring) {
					continue
				}
				seen[header.ID] = struct{}{}
				withTeam, err := archiveAddTeamName(raw, team.Name)
				if err != nil {
					return nil, err
				}
				channel.ID = header.ID
				channel.Name = header.Name
				channel.ChannelType = header.ChannelType
				channel.CreateAt = header.CreateAt
				channel.DeleteAt = header.DeleteAt
				channel.TeamName = team.Name
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

func archiveSyncPosts(ctx context.Context, apiClient *model.Client4, store *archive.Store, channels []*archiveChannel, isFullSync bool) error {
	state, err := store.LoadState()
	if err != nil {
		return err
	}
	if isFullSync {
		state = map[string]*archive.ChannelState{}
	}

	newPostCount, unreadableCount := 0, 0
	for index, channel := range channels {
		since := int64(0)
		if previous, isKnown := state[channel.ID]; isKnown {
			since = previous.LastCreateAt
		}

		collected, err := archiveReadChannel(ctx, apiClient, channel.ID, since)
		if err != nil {
			printer.PrintInfo("  cannot read %s/%s: %v", channel.TeamName, channel.Name, err)
			unreadableCount++
			continue
		}

		var fresh []*archivedPost
		for _, raw := range collected {
			header := &postHeader{}
			if err := json.Unmarshal(raw, header); err != nil {
				return fmt.Errorf("commands: parsing post: %w", err)
			}
			if header.CreateAt <= since {
				continue
			}
			fresh = append(fresh, &archivedPost{header: header, raw: raw})
		}
		sort.SliceStable(fresh, func(first, second int) bool {
			return fresh[first].header.CreateAt < fresh[second].header.CreateAt
		})

		lastCreateAt := since
		if len(fresh) > 0 {
			lines := make([]json.RawMessage, 0, len(fresh))
			var records []*archive.ArchivedFile
			for _, post := range fresh {
				lines = append(lines, post.raw)
				if post.header.CreateAt > lastCreateAt {
					lastCreateAt = post.header.CreateAt
				}
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
			// A channel read from the beginning replaces its file. Appending
			// would put a second copy of every post after the first.
			write := store.AppendPosts
			if since == 0 {
				write = store.ReplacePosts
			}
			if err := write(channel.TeamName, channel.Name, lines); err != nil {
				return err
			}
			if err := store.AppendFiles(records); err != nil {
				return err
			}
			newPostCount += len(fresh)
		}

		state[channel.ID] = &archive.ChannelState{
			TeamName:     channel.TeamName,
			ChannelName:  channel.Name,
			LastCreateAt: lastCreateAt,
			IsArchived:   channel.DeleteAt != 0,
		}
		if (index+1)%archiveStateInterval == 0 {
			if err := store.SaveState(state); err != nil {
				return err
			}
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

func archiveSyncUsers(ctx context.Context, apiClient *model.Client4, store *archive.Store) error {
	var users []*model.User
	for page := 0; ; page++ {
		batch, _, err := apiClient.GetUsers(ctx, page, archivePageSize, "")
		if err != nil {
			return fmt.Errorf("commands: listing users: %w", err)
		}
		users = append(users, batch...)
		if len(batch) < archivePageSize {
			break
		}
	}
	if err := store.SaveUsers(users); err != nil {
		return err
	}
	printer.PrintInfo("%d users recorded", len(users))
	return nil
}

func archiveSyncFiles(ctx context.Context, apiClient *model.Client4, store *archive.Store, me *model.User, fileScope string, maximumFileMegabytes float64) error {
	wanted, err := store.ReferencedFileIDs()
	if err != nil {
		return err
	}
	if fileScope == "mine" {
		mine, err := archiveOwnFileIDs(store, me.Id)
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
			unavailableCount++
			continue
		}
		if info.Size > maximumBytes {
			tooBigCount++
			continue
		}
		content, _, err := apiClient.DownloadFile(ctx, fileId, true)
		if err != nil {
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

// archiveOwnFileIDs reads the attachment ids on the user's own posts out of the
// archive, so "only my files" needs no further server calls.
func archiveOwnFileIDs(store *archive.Store, userId string) (map[string]struct{}, error) {
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
// worth retrying. A 403 or 404 is the server saying no, so it is returned as is.
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
			if response != nil && (response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound) {
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
