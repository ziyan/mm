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

	// A channel filter that matches nothing is an error, not an empty result
	// that looks like the archive holds nothing.
	if _, err := runCommand("archive", "search", directory, "cat", "--user", "", "--channel", "nosuchchannel"); err == nil {
		t.Errorf("a channel filter matching no channel should have failed")
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
