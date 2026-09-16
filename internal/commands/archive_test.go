package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIntegrationArchiveSyncAndSearch posts into a channel, archives it, and
// searches the archive. It then syncs again to check that the second run adds
// only what is new rather than a second copy of everything.
func TestIntegrationArchiveSyncAndSearch(t *testing.T) {
	skipIntegration(t)

	channelName := fmt.Sprintf("int-archive-%d", time.Now().UnixNano())
	if output, err := runCommand("channel", "create", channelName, "--display-name", "Integration Archive"); err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}

	needle := fmt.Sprintf("archiveneedle%d", time.Now().UnixNano())
	for _, message := range []string{"first archived message", needle, "third archived message"} {
		if output, err := runCommand("post", "create", channelName, message); err != nil {
			t.Fatalf("post create failed: %v\n%s", err, output)
		}
	}

	directory := t.TempDir()
	output, err := runCommand("archive", "sync", directory, "--only", channelName)
	if err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}
	if strings.Contains(output, "0 new posts") {
		t.Errorf("the first sync archived nothing: %s", output)
	}

	postsPath := filepath.Join(directory, "posts")
	entries, err := os.ReadDir(postsPath)
	if err != nil {
		t.Fatalf("reading %s: %v", postsPath, err)
	}
	if len(entries) == 0 {
		t.Fatalf("the sync wrote no team directory under %s", postsPath)
	}

	// Searching finds the needle, and says where it was written.
	output, err = runCommand("archive", "search", directory, needle)
	if err != nil {
		t.Fatalf("archive search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, needle) {
		t.Errorf("search did not find the needle: %s", output)
	}
	if !strings.Contains(output, channelName) {
		t.Errorf("search did not name the channel: %s", output)
	}

	// A search for something nobody posted finds nothing.
	output, err = runCommand("archive", "search", directory, "nothingpostedthisstring", "--regex=false", "--case-sensitive=false")
	if err != nil {
		t.Fatalf("archive search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "No matches") {
		t.Errorf("expected no matches, got: %s", output)
	}

	// The second sync must not re-fetch what is already on disk.
	output, err = runCommand("archive", "sync", directory, "--only", channelName)
	if err != nil {
		t.Fatalf("second archive sync failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "0 new posts") {
		t.Errorf("expected the second sync to add nothing, got: %s", output)
	}

	result, output, err := runCommandJSON("archive", "status", directory)
	if err != nil {
		t.Fatalf("archive status failed: %v\n%s", err, output)
	}
	status, isObject := result.(map[string]interface{})
	if !isObject {
		t.Fatalf("archive status did not return an object: %s", output)
	}
	// The channel also holds the system message for its own creation, so the
	// count is at least the three that were posted.
	if count, _ := status["posts"].(float64); int(count) < 3 {
		t.Errorf("expected at least 3 posts in the archive, status says %v", status["posts"])
	}

	// One more post, and the incremental sync picks up exactly that one.
	if output, err := runCommand("post", "create", channelName, "a fourth archived message"); err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	output, err = runCommand("archive", "sync", directory, "--only", channelName)
	if err != nil {
		t.Fatalf("third archive sync failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "1 new posts") {
		t.Errorf("expected exactly one new post, got: %s", output)
	}
}

// TestIntegrationArchiveSearchFilters checks the filters a search takes, which
// are what make a large archive usable.
func TestIntegrationArchiveSearchFilters(t *testing.T) {
	skipIntegration(t)

	channelName := fmt.Sprintf("int-archive-filter-%d", time.Now().UnixNano())
	if output, err := runCommand("channel", "create", channelName, "--display-name", "Integration Archive Filter"); err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}
	if output, err := runCommand("post", "create", channelName, "the cat sat on the MAT"); err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}

	directory := t.TempDir()
	if output, err := runCommand("archive", "sync", directory, "--only", channelName); err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}

	// Case insensitive by default, exact with --case-sensitive.
	output, err := runCommand("archive", "search", directory, "mat", "--case-sensitive=false", "--regex=false")
	if err != nil {
		t.Fatalf("archive search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "the cat sat") {
		t.Errorf("a case insensitive search missed the post: %s", output)
	}
	output, err = runCommand("archive", "search", directory, "mat", "--case-sensitive=true", "--regex=false")
	if err != nil {
		t.Fatalf("archive search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "No matches") {
		t.Errorf("--case-sensitive should not have matched MAT with mat: %s", output)
	}

	output, err = runCommand("archive", "search", directory, "c.t sat", "--regex=true", "--case-sensitive=false")
	if err != nil {
		t.Fatalf("archive search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "the cat sat") {
		t.Errorf("a regex search missed the post: %s", output)
	}

	// A date window that ends before the post was written excludes it.
	output, err = runCommand("archive", "search", directory, "cat", "--regex=false", "--case-sensitive=false", "--until", "2000-01-01")
	if err != nil {
		t.Fatalf("archive search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "No matches") {
		t.Errorf("--until should have excluded the post: %s", output)
	}

	// An unknown user is an error, not silently no matches.
	if _, err := runCommand("archive", "search", directory, "cat", "--until", "", "--user", "nobodyhere"); err == nil {
		t.Errorf("searching as an unknown user should have failed")
	}

	// A channel filter that matches nothing is a result of nothing, not an
	// error: the filter is the user's own, and an archive that does not hold
	// what they asked for is an answer.
	output, err = runCommand("archive", "search", directory, "cat", "--user", "", "--channel", "nosuchchannel")
	if err != nil {
		t.Fatalf("a channel filter matching nothing should not have failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "No matches") {
		t.Errorf("expected no matches for a channel filter matching nothing, got: %s", output)
	}

	// A directory that is not an archive is still an error.
	if _, err := runCommand("archive", "search", t.TempDir(), "cat", "--channel", "", "--user", ""); err == nil {
		t.Errorf("searching a directory that is not an archive should have failed")
	}
}

// TestIntegrationArchiveCatchesUpPastTheSinceCap checks that a channel further
// behind than one since response catches up in a single sync. The since
// endpoint caps its response at about a thousand posts and orders it by update
// time, so a sync built on it walks forward a capped batch at a time and steps
// over posts it never saw.
func TestIntegrationArchiveCatchesUpPastTheSinceCap(t *testing.T) {
	skipIntegration(t)

	channelName := fmt.Sprintf("int-archive-pages-%d", time.Now().UnixNano())
	if output, err := runCommand("channel", "create", channelName, "--display-name", "Integration Archive Pages"); err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}

	directory := t.TempDir()
	if output, err := runCommand("archive", "sync", directory, "--only", channelName); err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}

	// Past the cap the since endpoint puts on one response, so a sync built on
	// that endpoint stops short here and reports a round number instead.
	const postCount = 1050
	for index := 0; index < postCount; index++ {
		if output, err := runCommand("post", "create", channelName, fmt.Sprintf("message %d", index)); err != nil {
			t.Fatalf("post create failed: %v\n%s", err, output)
		}
	}

	output, err := runCommand("archive", "sync", directory, "--only", channelName)
	if err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, fmt.Sprintf("%d new posts", postCount)) {
		t.Errorf("expected all %d posts in one sync, got: %s", postCount, output)
	}

	// Every one of them is searchable, none archived twice.
	seen := map[string]int{}
	channelFiles, err := os.ReadDir(filepath.Join(directory, "posts"))
	if err != nil {
		t.Fatalf("reading the posts directory: %v", err)
	}
	for _, teamDirectory := range channelFiles {
		path := filepath.Join(directory, "posts", teamDirectory.Name(), channelName+".jsonl")
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
			post := struct {
				ID string `json:"id"`
			}{}
			if err := json.Unmarshal([]byte(line), &post); err != nil {
				t.Fatalf("parsing an archived line: %v", err)
			}
			seen[post.ID]++
		}
	}
	for postId, count := range seen {
		if count > 1 {
			t.Errorf("post %s archived %d times", postId, count)
		}
	}

	output, err = runCommand("archive", "sync", directory, "--only", channelName)
	if err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "0 new posts") {
		t.Errorf("expected the sync after a catch up to add nothing, got: %s", output)
	}
}

// TestIntegrationArchiveRepairFillsAGap checks that --since recovers posts an
// earlier sync never archived, without disturbing what is already on disk.
func TestIntegrationArchiveRepairFillsAGap(t *testing.T) {
	skipIntegration(t)

	channelName := fmt.Sprintf("int-archive-repair-%d", time.Now().UnixNano())
	if output, err := runCommand("channel", "create", channelName, "--display-name", "Integration Archive Repair"); err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}
	for index := 0; index < 6; index++ {
		if output, err := runCommand("post", "create", channelName, fmt.Sprintf("message %d", index)); err != nil {
			t.Fatalf("post create failed: %v\n%s", err, output)
		}
	}

	directory := t.TempDir()
	if output, err := runCommand("archive", "sync", directory, "--since", "", "--only", channelName); err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}

	// Cut two posts out of the middle, the shape an earlier sync's gap leaves.
	postsPath := ""
	teams, err := os.ReadDir(filepath.Join(directory, "posts"))
	if err != nil {
		t.Fatalf("reading the posts directory: %v", err)
	}
	for _, team := range teams {
		candidate := filepath.Join(directory, "posts", team.Name(), channelName+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			postsPath = candidate
		}
	}
	if postsPath == "" {
		t.Fatalf("no archived file for %s", channelName)
	}
	content, err := os.ReadFile(postsPath)
	if err != nil {
		t.Fatalf("reading the archived posts: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 archived lines, got %d", len(lines))
	}
	kept := append(append([]string{}, lines[:2]...), lines[4:]...)
	removed := len(lines) - len(kept)
	if err := os.WriteFile(postsPath, []byte(strings.Join(kept, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("writing the shortened archive: %v", err)
	}

	// A plain sync cannot see a gap behind its own mark.
	output, err := runCommand("archive", "sync", directory, "--since", "", "--only", channelName)
	if err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "0 new posts") {
		t.Errorf("a plain sync should not have reached behind its mark, got: %s", output)
	}

	// A repair does.
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	output, err = runCommand("archive", "sync", directory, "--only", channelName, "--since", yesterday)
	if err != nil {
		t.Fatalf("archive repair failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, fmt.Sprintf("%d new posts", removed)) {
		t.Errorf("expected the repair to recover %d posts, got: %s", removed, output)
	}

	content, err = os.ReadFile(postsPath)
	if err != nil {
		t.Fatalf("reading the repaired archive: %v", err)
	}
	seen := map[string]int{}
	var stamps []int64
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		post := struct {
			ID       string `json:"id"`
			CreateAt int64  `json:"create_at"`
		}{}
		if err := json.Unmarshal([]byte(line), &post); err != nil {
			t.Fatalf("parsing a repaired line: %v", err)
		}
		seen[post.ID]++
		stamps = append(stamps, post.CreateAt)
	}
	if len(seen) != len(lines) {
		t.Errorf("expected %d posts after the repair, got %d", len(lines), len(seen))
	}
	for postId, count := range seen {
		if count > 1 {
			t.Errorf("post %s appears %d times after the repair", postId, count)
		}
	}
	for index := 1; index < len(stamps); index++ {
		if stamps[index-1] > stamps[index] {
			t.Errorf("the repaired file is out of creation order at line %d", index)
			break
		}
	}
}

// TestIntegrationArchiveSyncKeepsUnseenChannels checks that a sync narrowed
// with --only does not drop the other channels from channels.json.
func TestIntegrationArchiveSyncKeepsUnseenChannels(t *testing.T) {
	skipIntegration(t)

	first := fmt.Sprintf("int-archive-keep-a-%d", time.Now().UnixNano())
	second := fmt.Sprintf("int-archive-keep-b-%d", time.Now().UnixNano())
	for _, channelName := range []string{first, second} {
		if output, err := runCommand("channel", "create", channelName, "--display-name", channelName); err != nil {
			t.Fatalf("channel create failed: %v\n%s", err, output)
		}
		if output, err := runCommand("post", "create", channelName, "hello"); err != nil {
			t.Fatalf("post create failed: %v\n%s", err, output)
		}
	}

	directory := t.TempDir()
	if output, err := runCommand("archive", "sync", directory, "--only", "int-archive-keep-"); err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}
	if output, err := runCommand("archive", "sync", directory, "--only", first); err != nil {
		t.Fatalf("narrowed archive sync failed: %v\n%s", err, output)
	}

	content, err := os.ReadFile(filepath.Join(directory, "channels.json"))
	if err != nil {
		t.Fatalf("reading channels.json: %v", err)
	}
	var channels []map[string]interface{}
	if err := json.Unmarshal(content, &channels); err != nil {
		t.Fatalf("parsing channels.json: %v", err)
	}
	names := map[string]bool{}
	for _, channel := range channels {
		if name, isString := channel["name"].(string); isString {
			names[name] = true
		}
	}
	if !names[first] || !names[second] {
		t.Errorf("a narrowed sync dropped a channel from channels.json, have %v", names)
	}
}

// TestIntegrationArchiveDirectAndGroupMessages checks that direct and group
// messages are archived under the pseudo-teams direct and group, named after
// the people in them, and that a search reaches them.
func TestIntegrationArchiveDirectAndGroupMessages(t *testing.T) {
	skipIntegration(t)

	needle := fmt.Sprintf("dmneedle%d", time.Now().UnixNano())
	if output, err := runCommand("dm", "send", "testuser2", needle); err != nil {
		t.Fatalf("dm send failed: %v\n%s", err, output)
	}
	groupNeedle := fmt.Sprintf("groupneedle%d", time.Now().UnixNano())
	if output, err := runCommand("dm", "group", "testuser2,testuser3", groupNeedle); err != nil {
		t.Fatalf("dm group failed: %v\n%s", err, output)
	}

	directory := t.TempDir()
	if output, err := runCommand("archive", "sync", directory, "--only", "testuser2"); err != nil {
		t.Fatalf("archive sync failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(directory, "posts", "direct", "testuser2.jsonl")); err != nil {
		t.Errorf("the direct message file is missing: %v", err)
	}
	groupFiles, _ := filepath.Glob(filepath.Join(directory, "posts", "group", "*testuser2*testuser3*.jsonl"))
	if len(groupFiles) == 0 {
		t.Errorf("no group message file named after its members under posts/group")
	}

	output, err := runCommand("archive", "search", directory, needle, "--channel", "", "--user", "", "--team", "direct")
	if err != nil {
		t.Fatalf("archive search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "direct/testuser2") {
		t.Errorf("search did not find the direct message: %s", output)
	}
	output, err = runCommand("archive", "search", directory, groupNeedle, "--channel", "", "--user", "", "--team", "")
	if err != nil {
		t.Fatalf("archive search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "group/") {
		t.Errorf("search did not find the group message: %s", output)
	}
}
