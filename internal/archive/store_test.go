package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"town-square":      "town-square",
		"engineering/team": "engineering_team",
		"a b c":            "a_b_c",
		"日本語":              "___",
	}
	for input, expected := range cases {
		if actual := SafeName(input); actual != expected {
			t.Errorf("SafeName(%q) = %q, expected %q", input, actual, expected)
		}
	}
	if length := len(SafeName(strings.Repeat("a", 300))); length != 120 {
		t.Errorf("SafeName did not cap a long name, got %d characters", length)
	}
}

func TestStateRoundTrip(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}

	empty, err := store.LoadState()
	if err != nil {
		t.Fatalf("loading an empty state: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected no state in a fresh archive, got %d entries", len(empty))
	}

	written := map[string]*ChannelState{
		"channel1": {TeamName: "engineering", ChannelName: "backend", LastCreateAt: 42, IsArchived: true},
	}
	if err := store.SaveState(written); err != nil {
		t.Fatalf("saving state: %v", err)
	}
	loaded, err := store.LoadState()
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if loaded["channel1"].LastCreateAt != 42 || !loaded["channel1"].IsArchived {
		t.Errorf("state did not survive the round trip: %+v", loaded["channel1"])
	}

	// The on-disk keys are a compatibility contract with older archives.
	content, err := os.ReadFile(filepath.Join(store.Directory(), stateFileName))
	if err != nil {
		t.Fatalf("reading state.json: %v", err)
	}
	for _, key := range []string{`"team"`, `"name"`, `"last"`, `"archived"`} {
		if !strings.Contains(string(content), key) {
			t.Errorf("state.json is missing the %s key: %s", key, content)
		}
	}
}

func TestMergeChannelsKeepsWhatItDidNotSee(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}

	first := []json.RawMessage{
		json.RawMessage(`{"id":"one","name":"backend","_team":"engineering"}`),
		json.RawMessage(`{"id":"two","name":"vision","_team":"engineering"}`),
	}
	if err := store.MergeChannels(first); err != nil {
		t.Fatalf("merging the first listing: %v", err)
	}

	// A sync narrowed with --only sees one channel. The other must survive.
	second := []json.RawMessage{
		json.RawMessage(`{"id":"one","name":"backend","_team":"engineering","display_name":"Backend"}`),
	}
	if err := store.MergeChannels(second); err != nil {
		t.Fatalf("merging the second listing: %v", err)
	}

	channels, err := store.LoadChannels()
	if err != nil {
		t.Fatalf("loading channels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels after a narrowed sync, got %d", len(channels))
	}
	if channels["one"].DisplayName != "Backend" {
		t.Errorf("the second listing did not replace the first: %+v", channels["one"])
	}
	if channels["two"].Name != "vision" {
		t.Errorf("a channel the narrowed sync did not see was dropped")
	}
}

func TestAppendAndReadPosts(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}

	lines := []json.RawMessage{
		json.RawMessage(`{"id":"p1","create_at":1,"message":"first"}`),
		json.RawMessage(`{"id":"p2","create_at":2,"message":"second"}`),
	}
	if err := store.AppendPosts("engineering", "backend", lines); err != nil {
		t.Fatalf("appending posts: %v", err)
	}
	if err := store.AppendPosts("engineering", "backend",
		[]json.RawMessage{json.RawMessage(`{"id":"p3","create_at":3,"message":"third"}`)}); err != nil {
		t.Fatalf("appending more posts: %v", err)
	}

	channelFiles, err := store.ChannelFiles()
	if err != nil {
		t.Fatalf("listing channel files: %v", err)
	}
	if len(channelFiles) != 1 {
		t.Fatalf("expected 1 channel file, got %d", len(channelFiles))
	}
	if channelFiles[0].TeamName != "engineering" || channelFiles[0].ChannelName != "backend" {
		t.Errorf("channel file named wrong: %+v", channelFiles[0])
	}

	var messages []string
	if err := ReadPosts(channelFiles[0].Path, func(line []byte) bool {
		post := struct {
			Message string `json:"message"`
		}{}
		if err := json.Unmarshal(line, &post); err != nil {
			t.Errorf("parsing an archived line: %v", err)
		}
		messages = append(messages, post.Message)
		return true
	}); err != nil {
		t.Fatalf("reading posts: %v", err)
	}
	if strings.Join(messages, ",") != "first,second,third" {
		t.Errorf("posts came back as %v, expected them oldest first and appended", messages)
	}
}

func TestReadPostsHandlesLongLines(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	// Longer than bufio.Scanner's token limit, which is why ReadPosts does not use one.
	long := strings.Repeat("x", 128*1024)
	line, err := json.Marshal(map[string]string{"id": "p1", "message": long})
	if err != nil {
		t.Fatalf("encoding a long post: %v", err)
	}
	if err := store.AppendPosts("team", "channel", []json.RawMessage{line}); err != nil {
		t.Fatalf("appending a long post: %v", err)
	}

	read := 0
	if err := ReadPosts(store.PostsPath("team", "channel"), func(line []byte) bool {
		post := struct {
			Message string `json:"message"`
		}{}
		if err := json.Unmarshal(line, &post); err != nil {
			t.Fatalf("a long line did not come back whole: %v", err)
		}
		if len(post.Message) != len(long) {
			t.Errorf("message came back %d bytes, expected %d", len(post.Message), len(long))
		}
		read++
		return true
	}); err != nil {
		t.Fatalf("reading a long post: %v", err)
	}
	if read != 1 {
		t.Errorf("expected 1 post, read %d", read)
	}
}

func TestReferencedFileIDs(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	if err := store.AppendFiles([]*ArchivedFile{
		{FileID: "f1", PostID: "p1", ChannelName: "backend", TeamName: "engineering", CreateAt: 1},
		{FileID: "f2", PostID: "p2", ChannelName: "backend", TeamName: "engineering", CreateAt: 2},
	}); err != nil {
		t.Fatalf("appending file records: %v", err)
	}
	fileIds, err := store.ReferencedFileIDs()
	if err != nil {
		t.Fatalf("reading file records: %v", err)
	}
	if len(fileIds) != 2 {
		t.Errorf("expected 2 attachment ids, got %d", len(fileIds))
	}
	if _, isKnown := fileIds["f1"]; !isKnown {
		t.Errorf("f1 is missing from the attachment index")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory in this environment")
	}
	expanded, err := ExpandPath("~/mattermost-archive")
	if err != nil {
		t.Fatalf("expanding a path: %v", err)
	}
	if expanded != filepath.Join(home, "mattermost-archive") {
		t.Errorf("ExpandPath gave %q", expanded)
	}
	if expanded, _ := ExpandPath("/tmp/archive"); expanded != "/tmp/archive" {
		t.Errorf("an absolute path should be left alone, got %q", expanded)
	}
}

func TestNewestPostAndAppendAfterAPartialLine(t *testing.T) {
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatalf("creating the archive: %v", err)
	}
	if newest, endsWithNewline, err := store.NewestPost("team", "channel"); err != nil || newest != nil || !endsWithNewline {
		t.Fatalf("a missing file should read as empty, got %q %v %v", newest, endsWithNewline, err)
	}

	long := strings.Repeat("y", 200*1024) // longer than one backward chunk
	first, _ := json.Marshal(map[string]string{"id": "p1", "message": long})
	second, _ := json.Marshal(map[string]string{"id": "p2", "message": "short"})
	if err := store.AppendPosts("team", "channel", []json.RawMessage{first, second}); err != nil {
		t.Fatalf("appending: %v", err)
	}
	newest, endsWithNewline, err := store.NewestPost("team", "channel")
	if err != nil || !endsWithNewline || string(newest) != string(second) {
		t.Fatalf("expected the second post as the newest, got %.40q %v %v", newest, endsWithNewline, err)
	}
	// Only the long line, so the walk back reaches the start of the file.
	if err := os.WriteFile(store.PostsPath("team", "channel"), append(first, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if newest, _, err := store.NewestPost("team", "channel"); err != nil || len(newest) != len(first) {
		t.Fatalf("expected the long post back whole, got %d bytes, %v", len(newest), err)
	}

	// A file cut short mid-line: the next append must start on a new line.
	if err := os.WriteFile(store.PostsPath("team", "channel"), []byte(`{"id":"p1","mess`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendPosts("team", "channel", []json.RawMessage{second}); err != nil {
		t.Fatalf("appending after a partial line: %v", err)
	}
	var lines []string
	if err := ReadPosts(store.PostsPath("team", "channel"), func(line []byte) bool {
		lines = append(lines, string(line))
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[1] != string(second) {
		t.Fatalf("expected the partial line and then the post on its own line, got %q", lines)
	}
}

func TestOpenRequiresAnArchive(t *testing.T) {
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("an empty directory must not open as an archive")
	}
	store, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveState(map[string]*ChannelState{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(store.Directory()); err != nil {
		t.Fatalf("a synced directory must open: %v", err)
	}
}
