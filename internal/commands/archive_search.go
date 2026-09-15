package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/archive"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

// searchedPost is one archived post, parsed far enough to filter and print it.
type searchedPost struct {
	ID        string `json:"id"`
	CreateAt  int64  `json:"create_at"`
	UserID    string `json:"user_id"`
	Message   string `json:"message"`
	PostType  string `json:"type"`
	RootID    string `json:"root_id"`
	ChannelID string `json:"channel_id"`
}

// searchMatch is what gets printed.
type searchMatch struct {
	TeamName    string `json:"team"`
	ChannelName string `json:"channel"`
	Username    string `json:"username"`
	CreateAt    int64  `json:"create_at"`
	Timestamp   string `json:"timestamp"`
	Message     string `json:"message"`
	PostID      string `json:"post_id"`
	RootID      string `json:"root_id,omitempty"`
	Permalink   string `json:"permalink,omitempty"`
}

func archiveSearchRun(command *cobra.Command, arguments []string) error {
	store, err := archive.Open(arguments[0])
	if err != nil {
		return err
	}
	query := arguments[1]

	channelSubstring, _ := command.Flags().GetString("channel")
	username, _ := command.Flags().GetString("user")
	sinceText, _ := command.Flags().GetString("since")
	untilText, _ := command.Flags().GetString("until")
	limit, _ := command.Flags().GetInt("limit")
	isRegex, _ := command.Flags().GetBool("regex")
	isCaseSensitive, _ := command.Flags().GetBool("case-sensitive")
	isOldestFirst, _ := command.Flags().GetBool("oldest-first")
	teamSubstring, _ := command.Flags().GetString("team")

	since, err := parseSearchDate(sinceText, false)
	if err != nil {
		return err
	}
	until, err := parseSearchDate(untilText, true)
	if err != nil {
		return err
	}

	var pattern *regexp.Regexp
	if isRegex {
		expression := query
		if !isCaseSensitive {
			expression = "(?i)" + expression
		}
		pattern, err = regexp.Compile(expression)
		if err != nil {
			return fmt.Errorf("commands: compiling the query: %w", err)
		}
	}

	usernames, err := store.LoadUsernames()
	if err != nil {
		return err
	}
	wantedUserId := ""
	if username != "" {
		wanted := strings.TrimPrefix(username, "@")
		for userId, name := range usernames {
			if strings.EqualFold(name, wanted) {
				wantedUserId = userId
				break
			}
		}
		if wantedUserId == "" {
			return fmt.Errorf("commands: no user named %q in the archive, run mm archive sync to record users", wanted)
		}
	}

	channelFiles, err := store.ChannelFiles()
	if err != nil {
		return err
	}
	var wantedFiles []*archive.ChannelFile
	for _, channelFile := range channelFiles {
		if channelSubstring != "" && !strings.Contains(channelFile.ChannelName, channelSubstring) {
			continue
		}
		if teamSubstring != "" && !strings.Contains(channelFile.TeamName, teamSubstring) {
			continue
		}
		wantedFiles = append(wantedFiles, channelFile)
	}
	if len(wantedFiles) == 0 {
		return fmt.Errorf("commands: no archived channels match, is %s an archive directory", store.Directory())
	}

	// A cheap substring test on the raw line decides whether a post is worth
	// parsing, which is what keeps a scan of the whole archive reasonable.
	lowerQuery := []byte(strings.ToLower(query))
	rawQuery := []byte(query)

	matches := searchChannelFiles(wantedFiles, func(line []byte) *searchedPost {
		if isRegex {
			if !pattern.Match(line) {
				return nil
			}
		} else if isCaseSensitive {
			if !bytes.Contains(line, rawQuery) {
				return nil
			}
		} else if !bytes.Contains(bytes.ToLower(line), lowerQuery) {
			return nil
		}
		post := &searchedPost{}
		if json.Unmarshal(line, post) != nil {
			return nil
		}
		if post.PostType != "" {
			return nil // a join, a leave or a header change, which nobody wrote
		}
		if wantedUserId != "" && post.UserID != wantedUserId {
			return nil
		}
		if since > 0 && post.CreateAt < since {
			return nil
		}
		if until > 0 && post.CreateAt > until {
			return nil
		}
		if !matchesMessage(post.Message, query, pattern, isRegex, isCaseSensitive) {
			return nil
		}
		return post
	})

	serverUrl := ""
	if _, server, err := client.New(); err == nil {
		serverUrl = strings.TrimRight(server.URL, "/")
		if !strings.HasPrefix(serverUrl, "http") {
			serverUrl = "https://" + serverUrl
		}
	}

	sort.SliceStable(matches, func(first, second int) bool {
		if isOldestFirst {
			return matches[first].post.CreateAt < matches[second].post.CreateAt
		}
		return matches[first].post.CreateAt > matches[second].post.CreateAt
	})
	totalCount := len(matches)
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	results := make([]*searchMatch, 0, len(matches))
	for _, match := range matches {
		name := usernames[match.post.UserID]
		if name == "" {
			name = match.post.UserID
		}
		result := &searchMatch{
			TeamName:    match.teamName,
			ChannelName: match.channelName,
			Username:    name,
			CreateAt:    match.post.CreateAt,
			Timestamp:   time.UnixMilli(match.post.CreateAt).Format("2006-01-02 15:04"),
			Message:     match.post.Message,
			PostID:      match.post.ID,
			RootID:      match.post.RootID,
		}
		if serverUrl != "" {
			result.Permalink = fmt.Sprintf("%s/%s/pl/%s", serverUrl, match.teamName, match.post.ID)
		}
		results = append(results, result)
	}

	if printer.JSONOutput {
		printer.PrintJSON(map[string]interface{}{
			"query":   query,
			"total":   totalCount,
			"matches": results,
		})
		return nil
	}

	if totalCount == 0 {
		printer.PrintInfo("No matches.")
		return nil
	}
	for _, result := range results {
		printer.PrintInfo("%s  %s/%s  @%s", result.Timestamp, result.TeamName, result.ChannelName, result.Username)
		for _, line := range strings.Split(strings.TrimRight(result.Message, "\n"), "\n") {
			printer.PrintInfo("    %s", line)
		}
		if result.Permalink != "" {
			printer.PrintInfo("    %s", result.Permalink)
		}
		printer.PrintInfo("")
	}
	if limit > 0 && totalCount > limit {
		printer.PrintInfo("%d matches, showing %d. Raise --limit to see more.", totalCount, limit)
	} else {
		printer.PrintInfo("%d matches.", totalCount)
	}
	return nil
}

type locatedPost struct {
	teamName    string
	channelName string
	post        *searchedPost
}

// searchChannelFiles reads the channel files in parallel. The archive is large
// enough that one goroutine per core is the difference between a few seconds
// and most of a minute.
func searchChannelFiles(channelFiles []*archive.ChannelFile, test func(line []byte) *searchedPost) []*locatedPost {
	workerCount := runtime.NumCPU()
	if workerCount > len(channelFiles) {
		workerCount = len(channelFiles)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	work := make(chan *archive.ChannelFile)
	var waitGroup sync.WaitGroup
	var lock sync.Mutex
	var matches []*locatedPost
	collect := func(found []*locatedPost) {
		lock.Lock()
		defer lock.Unlock()
		matches = append(matches, found...)
	}

	for worker := 0; worker < workerCount; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for channelFile := range work {
				var found []*locatedPost
				_ = archive.ReadPosts(channelFile.Path, func(line []byte) bool {
					if post := test(line); post != nil {
						found = append(found, &locatedPost{
							teamName:    channelFile.TeamName,
							channelName: channelFile.ChannelName,
							post:        post,
						})
					}
					return true
				})
				if len(found) > 0 {
					collect(found)
				}
			}
		}()
	}
	for _, channelFile := range channelFiles {
		work <- channelFile
	}
	close(work)
	waitGroup.Wait()
	return matches
}

// matchesMessage re-tests the match against the message text alone. The cheap
// test that got us here ran over the whole JSON line, which also carries ids,
// props and attachment names.
func matchesMessage(message, query string, pattern *regexp.Regexp, isRegex, isCaseSensitive bool) bool {
	if isRegex {
		return pattern.MatchString(message)
	}
	if isCaseSensitive {
		return strings.Contains(message, query)
	}
	return strings.Contains(strings.ToLower(message), strings.ToLower(query))
}

func parseSearchDate(text string, isEndOfDay bool) (int64, error) {
	if text == "" {
		return 0, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", text, time.Local)
	if err != nil {
		return 0, fmt.Errorf("commands: parsing %q as a date (YYYY-MM-DD): %w", text, err)
	}
	if isEndOfDay {
		parsed = parsed.Add(24*time.Hour - time.Millisecond)
	}
	return parsed.UnixMilli(), nil
}
