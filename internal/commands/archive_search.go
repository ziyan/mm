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
	"unicode/utf8"

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

	matcher, err := newMessageMatcher(query, isRegex, isCaseSensitive)
	if err != nil {
		return err
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

	found, err := searchChannelFiles(wantedFiles, func(line []byte) *searchedPost {
		if matcher.canPrefilter && !matcher.matches(line) {
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
		if !matcher.matches([]byte(post.Message)) {
			return nil
		}
		return post
	})
	if err != nil {
		return err
	}

	serverUrl := ""
	if apiClient, _, err := client.New(); err == nil {
		serverUrl = strings.TrimRight(apiClient.URL, "/")
	}

	sort.SliceStable(found, func(first, second int) bool {
		if isOldestFirst {
			return found[first].post.CreateAt < found[second].post.CreateAt
		}
		return found[first].post.CreateAt > found[second].post.CreateAt
	})
	totalCount := len(found)
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}

	results := make([]*searchMatch, 0, len(found))
	for _, match := range found {
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
// and most of a minute. A file that cannot be read fails the search rather
// than quietly contributing nothing, since "no matches" would be a lie.
func searchChannelFiles(channelFiles []*archive.ChannelFile, test func(line []byte) *searchedPost) ([]*locatedPost, error) {
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
	var firstError error
	collect := func(found []*locatedPost, err error) {
		lock.Lock()
		defer lock.Unlock()
		matches = append(matches, found...)
		if err != nil && firstError == nil {
			firstError = err
		}
	}

	for worker := 0; worker < workerCount; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for channelFile := range work {
				var found []*locatedPost
				err := archive.ReadPosts(channelFile.Path, func(line []byte) bool {
					if post := test(line); post != nil {
						found = append(found, &locatedPost{
							teamName:    channelFile.TeamName,
							channelName: channelFile.ChannelName,
							post:        post,
						})
					}
					return true
				})
				if len(found) > 0 || err != nil {
					collect(found, err)
				}
			}
		}()
	}
	for _, channelFile := range channelFiles {
		work <- channelFile
	}
	close(work)
	waitGroup.Wait()
	return matches, firstError
}

// messageMatcher is the one test a search applies to a post's message. If
// canPrefilter holds, the search also runs it over the raw JSON line first,
// and parses a post only when it may match.
type messageMatcher struct {
	matches      func(text []byte) bool
	canPrefilter bool
}

// newMessageMatcher builds the test for a query. A plain query is a byte
// search, which runs through the archive several times faster than a regular
// expression compiled from the same text.
//
// The raw-line prefilter is only sound when the JSON escaping of the message
// cannot change the answer: a quote, backslash, control character, or a
// character an encoder may write as \uXXXX, is not stored as itself. A
// regular expression is never prefiltered: its anchors and classes would bind
// to the line rather than the message, and parsing every post is cheaper than
// running it over every line anyway.
func newMessageMatcher(query string, isRegex, isCaseSensitive bool) (*messageMatcher, error) {
	if isRegex {
		expression := query
		if !isCaseSensitive {
			expression = "(?i)" + expression
		}
		pattern, err := regexp.Compile(expression)
		if err != nil {
			return nil, fmt.Errorf("commands: compiling the query: %w", err)
		}
		// A case-sensitive expression opening with a literal cannot match a
		// message whose line lacks that literal, so the line is tested for
		// it first. There is no such prefix once case is folded.
		prefix, _ := pattern.LiteralPrefix()
		if prefix != "" && survivesJsonEscaping(prefix) {
			wanted := []byte(prefix)
			return &messageMatcher{
				matches: func(text []byte) bool {
					return bytes.Contains(text, wanted) && pattern.Match(text)
				},
				canPrefilter: true,
			}, nil
		}
		return &messageMatcher{matches: pattern.Match}, nil
	}

	canPrefilter := survivesJsonEscaping(query)
	if isCaseSensitive {
		wanted := []byte(query)
		return &messageMatcher{
			matches:      func(text []byte) bool { return bytes.Contains(text, wanted) },
			canPrefilter: canPrefilter,
		}, nil
	}
	wanted := []byte(strings.ToLower(query))
	if !isAscii(query) {
		// Case folding can change the length of a character outside ASCII,
		// which the window comparison below cannot follow, so both sides are
		// lowered in full. Rare enough that the copy per line is acceptable.
		return &messageMatcher{
			matches:      func(text []byte) bool { return bytes.Contains(bytes.ToLower(text), wanted) },
			canPrefilter: canPrefilter,
		}, nil
	}
	return &messageMatcher{
		matches:      func(text []byte) bool { return containsFold(text, wanted) },
		canPrefilter: canPrefilter,
	}, nil
}

// survivesJsonEscaping reports whether text is stored as itself inside a JSON
// string. A quote, backslash or control character is not, and nor is a
// character an encoder may write as \uXXXX.
func survivesJsonEscaping(text string) bool {
	if strings.ContainsAny(text, "\"\\<>&\u2028\u2029") {
		return false
	}
	return !strings.ContainsFunc(text, func(character rune) bool { return character < ' ' || character == 0x7f })
}

func isAscii(text string) bool {
	return !strings.ContainsFunc(text, func(character rune) bool { return character >= utf8.RuneSelf })
}

// containsFold reports whether text holds query ignoring case. It avoids the
// lowercase copy of text that bytes.ToLower would make for every line in the
// archive. The caller lowercases query first, and keeps it to ASCII. It
// compares with bytes.EqualFold over windows of the query's length, so a
// character whose case variants differ in length, such as the Kelvin sign
// for k, does not match its ASCII counterpart.
func containsFold(text, query []byte) bool {
	if len(query) == 0 {
		return true
	}
	first := query[0]
	upper := first
	if 'a' <= first && first <= 'z' {
		upper = first - ('a' - 'A')
	}
	for index := 0; index+len(query) <= len(text); index++ {
		character := text[index]
		if character != first && character != upper && character < utf8.RuneSelf {
			continue
		}
		if bytes.EqualFold(text[index:index+len(query)], query) {
			return true
		}
	}
	return false
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
