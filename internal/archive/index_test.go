package archive

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func indexTestPost(id string, createAt int64, message string) json.RawMessage {
	encoded, err := json.Marshal(map[string]interface{}{
		"id": id, "create_at": createAt, "user_id": "user1", "channel_id": "channel1",
		"message": message, "props": map[string]interface{}{"padding": strings.Repeat("x", 500)},
	})
	if err != nil {
		panic(err)
	}
	return encoded
}

func channelFileFor(store *Store, teamName, channelName string) *ChannelFile {
	return &ChannelFile{TeamName: teamName, ChannelName: channelName, Path: store.PostsPath(teamName, channelName)}
}

// A new channel is indexed from its first post, the index keeps what a search
// reads and drops what it does not, and each append keeps it current.
func TestIndexFollowsAppends(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendPosts("team", "channel", []json.RawMessage{indexTestPost("p1", 1, "first <b>&")}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendPosts("team", "channel", []json.RawMessage{indexTestPost("p2", 2, "second")}); err != nil {
		t.Fatal(err)
	}
	path, isIndexed := store.SearchPath(channelFileFor(store, "team", "channel"))
	if !isIndexed {
		t.Fatal("a channel written by appends should have a current index")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "padding") {
		t.Error("the index should leave out fields a search does not read")
	}
	// Written as the server writes it, so the quick test on a raw line agrees.
	if !strings.Contains(string(content), "first <b>&") {
		t.Errorf("the message should be kept unescaped, got %s", content)
	}
	if lineCount, _ := CountLines(path); lineCount != 2 {
		t.Errorf("expected 2 index lines, got %d", lineCount)
	}
	postsSize, _ := fileSize(store.PostsPath("team", "channel"))
	if int64(len(content))*5 > postsSize {
		t.Errorf("the index is %d bytes against %d of posts, which is no saving", len(content), postsSize)
	}
}

// An archive from before indexes existed has posts and no index. Appending to
// it must not create an index of just the new posts that then claims to be
// complete, or a search would silently skip everything older.
func TestAppendDoesNotMakeAMissingIndexLookCurrent(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendPosts("team", "channel", []json.RawMessage{indexTestPost("p1", 1, "old")}); err != nil {
		t.Fatal(err)
	}
	// Take the index away, as for an archive written by an older version.
	if err := os.RemoveAll(store.IndexDirectory()); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendPosts("team", "channel", []json.RawMessage{indexTestPost("p2", 2, "new")}); err != nil {
		t.Fatal(err)
	}
	path, isIndexed := store.SearchPath(channelFileFor(store, "team", "channel"))
	if isIndexed {
		t.Fatal("an index missing older posts must not be taken as current")
	}
	if path != store.PostsPath("team", "channel") {
		t.Errorf("a search should fall back to the posts, got %s", path)
	}

	if err := store.RebuildIndex("team", "channel"); err != nil {
		t.Fatal(err)
	}
	path, isIndexed = store.SearchPath(channelFileFor(store, "team", "channel"))
	if !isIndexed {
		t.Fatal("a rebuilt index should be current")
	}
	if lineCount, _ := CountLines(path); lineCount != 2 {
		t.Errorf("the rebuilt index should hold both posts, got %d", lineCount)
	}
}

// Posts changed behind the index's back, by hand or by a sync cut short, make
// it stale, and a stale index is ignored.
func TestIndexGoesStaleWhenThePostsChange(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendPosts("team", "channel", []json.RawMessage{indexTestPost("p1", 1, "one")}); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(store.PostsPath("team", "channel"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write(append(indexTestPost("p2", 2, "added by hand"), '\n'))
	_ = file.Close()

	if _, isIndexed := store.SearchPath(channelFileFor(store, "team", "channel")); isIndexed {
		t.Fatal("an index older than its posts must not be used")
	}
}

// A merge puts posts in the middle, which an append cannot follow.
func TestMergeRebuildsTheIndex(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendPosts("team", "channel", []json.RawMessage{
		indexTestPost("p1", 1, "one"), indexTestPost("p3", 3, "three"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MergePosts("team", "channel", []json.RawMessage{indexTestPost("p2", 2, "two")}); err != nil {
		t.Fatal(err)
	}
	path, isIndexed := store.SearchPath(channelFileFor(store, "team", "channel"))
	if !isIndexed {
		t.Fatal("a merge should leave a current index")
	}
	content, _ := os.ReadFile(path)
	order := []int{strings.Index(string(content), `"p1"`), strings.Index(string(content), `"p2"`), strings.Index(string(content), `"p3"`)}
	if order[0] < 0 || order[0] >= order[1] || order[1] >= order[2] {
		t.Errorf("the index should follow the merged order, got %s", content)
	}
}

func TestCountLines(t *testing.T) {
	directory := t.TempDir()
	for content, expected := range map[string]int64{"": 0, "a\n": 1, "a\nb\n": 2, "a\nb": 2} {
		path := directory + "/count"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if lineCount, err := CountLines(path); err != nil || lineCount != expected {
			t.Errorf("CountLines(%q) = %d, %v; expected %d", content, lineCount, err, expected)
		}
	}
}
