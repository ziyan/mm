package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ziyan/mm/internal/archive"
)

func rawPost(id string, createAt int64, message string) json.RawMessage {
	encoded, err := json.Marshal(map[string]interface{}{
		"id":         id,
		"create_at":  createAt,
		"user_id":    "user1",
		"message":    message,
		"channel_id": "channel1",
	})
	if err != nil {
		panic(err)
	}
	return encoded
}

// TestArchiveWritePostsStoresEachPostOnce covers the crash case: state.json
// was not saved after a file was written, so the next run reads the same
// posts again. It also covers a re-read that reaches back past the mark.
func TestArchiveWritePostsStoresEachPostOnce(t *testing.T) {
	store, err := archive.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	channel := &archiveChannel{ID: "channel1", Name: "general-chat", TeamName: "example-team"}

	first := map[string]json.RawMessage{
		"post2": rawPost("post2", 200, "second"),
		"post3": rawPost("post3", 300, "third"),
	}
	written, err := archiveWritePosts(store, channel, first, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.fresh) != 2 {
		t.Fatalf("expected 2 fresh posts, got %d", len(written.fresh))
	}
	if written.newestCreateAt != 300 {
		t.Fatalf("expected the mark at the newest post, got %d", written.newestCreateAt)
	}

	// The same posts again, as after a crash before SaveState, plus one newer.
	second := map[string]json.RawMessage{
		"post2": rawPost("post2", 200, "second"),
		"post3": rawPost("post3", 300, "third"),
		"post4": rawPost("post4", 400, "fourth"),
	}
	written, err = archiveWritePosts(store, channel, second, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.fresh) != 1 || written.fresh[0].header.ID != "post4" {
		t.Fatalf("expected only post4 to be fresh, got %+v", written.fresh)
	}

	// A repair reaching back before what is on disk: merged, in order.
	repair := map[string]json.RawMessage{
		"post1": rawPost("post1", 100, "first"),
		"post3": rawPost("post3", 300, "third"),
	}
	written, err = archiveWritePosts(store, channel, repair, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.fresh) != 1 || written.fresh[0].header.ID != "post1" {
		t.Fatalf("expected only post1 to be fresh, got %+v", written.fresh)
	}

	var ids []string
	err = store.ScanPosts("example-team", "general-chat", func(line []byte) bool {
		header := &postHeader{}
		if err := json.Unmarshal(line, header); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, header.ID)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ids) != "[post1 post2 post3 post4]" {
		t.Fatalf("expected posts once each, oldest first, got %v", ids)
	}
	if _, err := os.Stat(store.PostsPath("example-team", "general-chat") + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file left behind: %v", err)
	}
}

// TestMessageMatcherSeesThroughJSONEscaping checks that a query holding a
// character JSON escapes still finds the post, which the raw-line prefilter
// alone could not, and that the prefilter is declined for such a query.
func TestMessageMatcherSeesThroughJSONEscaping(t *testing.T) {
	matcher, err := newMessageMatcher(`say "hi" & <bye>`, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if matcher.prefilter != nil {
		t.Fatal("a query holding a quote must not be tested against the raw line")
	}
	line := rawPost("post1", 100, `Please SAY "hi" & <bye> now`)
	if matcher.matches(line) {
		t.Fatalf("the raw line must not match, its message is escaped: %s", line)
	}
	post := &searchedPost{}
	if err := json.Unmarshal(line, post); err != nil {
		t.Fatal(err)
	}
	if !matcher.matches([]byte(post.Message)) {
		t.Fatalf("expected the decoded message to match: %q", post.Message)
	}

	plain, err := newMessageMatcher("say", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if plain.prefilter == nil || !plain.prefilter(line) {
		t.Fatal("a plain query is tested against the raw line first, and must match it")
	}
	exact, err := newMessageMatcher("say", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if exact.matches([]byte(post.Message)) {
		t.Fatal("--case-sensitive must not match SAY with say")
	}

	anchored, err := newMessageMatcher(`^please`, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if anchored.prefilter != nil {
		t.Fatal("a regular expression with no literal prefix must not touch the raw line")
	}
	if anchored.matches(line) || !anchored.matches([]byte(post.Message)) {
		t.Fatal("a regex anchor must bind to the message, not the line")
	}
}

func TestContainsFold(t *testing.T) {
	cases := []struct {
		text, query string
		expected    bool
	}{
		{"The Cat sat", "cat", true},
		{"The Cat sat", "CAT SAT", true},
		{"The Cat sat", "dog", false},
		{"", "a", false},
		{"abc", "", true},
		{"Straße", "strasse", false},
		{"cat", "cats", false},
	}
	for _, testCase := range cases {
		actual := containsFold([]byte(testCase.text), []byte(testCase.query))
		if actual != testCase.expected {
			t.Errorf("containsFold(%q, %q) = %v, expected %v", testCase.text, testCase.query, actual, testCase.expected)
		}
	}
}

// TestArchiveWritePostsDetectsAStaleMark covers the ordinary append path after
// a run was cut short between writing posts and saving state.json: the mark
// still says the file ends earlier than it does, and the posts read again must
// not be stored a second time.
func TestArchiveWritePostsDetectsAStaleMark(t *testing.T) {
	store, err := archive.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	channel := &archiveChannel{ID: "channel1", Name: "general-chat", TeamName: "example-team"}
	batch := map[string]json.RawMessage{
		"post2": rawPost("post2", 200, "second"),
		"post3": rawPost("post3", 300, "third"),
	}
	if _, err := archiveWritePosts(store, channel, batch, 100, false); err != nil {
		t.Fatal(err)
	}
	// The mark was never advanced past 100, so the same read happens again.
	batch["post4"] = rawPost("post4", 400, "fourth")
	written, err := archiveWritePosts(store, channel, batch, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.fresh) != 1 || written.fresh[0].header.ID != "post4" {
		t.Fatalf("expected only post4 to be fresh after a stale mark, got %d posts", len(written.fresh))
	}
	count := 0
	if err := store.ScanPosts("example-team", "general-chat", func(line []byte) bool { count++; return true }); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 posts on disk, got %d", count)
	}
}

func TestMessageMatcherFoldsCaseOutsideASCII(t *testing.T) {
	matcher, err := newMessageMatcher("école", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.matches([]byte("ÉCOLE")) {
		t.Fatal("expected a non-ASCII query to match ignoring case")
	}
	istanbul, err := newMessageMatcher("İstanbul", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !istanbul.matches([]byte("İstanbul")) {
		t.Fatal("expected the verbatim text to match itself")
	}
	prefixed, err := newMessageMatcher(`dead[a-z]+ in`, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if prefixed.prefilter == nil || !prefixed.matches([]byte("a deadlock in the works")) || prefixed.matches([]byte("a lock in")) {
		t.Fatal("a case-sensitive regex with a literal prefix is prefiltered on that prefix")
	}
}

func TestArchiveDirectChannelName(t *testing.T) {
	usernames := map[string]string{"me": "ziyan", "other": "alice"}
	if name := archiveDirectChannelName("me__other", "me", usernames); name != "alice" {
		t.Errorf("expected the other person's username, got %q", name)
	}
	if name := archiveDirectChannelName("other__me", "me", usernames); name != "alice" {
		t.Errorf("the order of the ids must not matter, got %q", name)
	}
	if name := archiveDirectChannelName("me__me", "me", usernames); name != "ziyan" {
		t.Errorf("a message to oneself is named after oneself, got %q", name)
	}
	if name := archiveDirectChannelName("me__gone", "me", usernames); name != "gone" {
		t.Errorf("an unknown user falls back to the id, got %q", name)
	}
}

// TestArchiveDisambiguateName covers two channels wanting one name, which the
// server allows: a group message's display name is capped at 64 characters, so
// two groups whose members agree that far are named the same.
func TestArchiveDisambiguateName(t *testing.T) {
	first := archiveDisambiguateName("alice, bob, carol", "ch4mbo6asid9pmih5g314x6nwh")
	second := archiveDisambiguateName("alice, bob, carol", "q4okb5tfhfg7defz96mnbyaste")
	if first == second {
		t.Fatalf("two channels were given the same name: %q", first)
	}
	if !strings.HasPrefix(first, "alice__bob__carol-") {
		t.Errorf("the name should keep its readable part, got %q", first)
	}
	// A name already at the cap must not lose its suffix to the cap.
	long := archiveDisambiguateName(strings.Repeat("a", 200), "ch4mbo6asid9pmih5g314x6nwh")
	if length := len(archive.SafeName(long)); length != len(long) || length > archive.MaximumNameLength {
		t.Errorf("a long name came out %d characters and does not survive SafeName", len(long))
	}
	if !strings.HasSuffix(long, "-ch4mbo6a") {
		t.Errorf("the suffix was cut off a long name: %q", long)
	}
}

// TestMessageMatcherPrefilterIsOnlyANecessaryCondition guards the mistake of
// running the whole test over the raw line: an anchor or a class in the
// pattern would bind to the line, which holds the post's ids and props as well
// as its message, and the post would be dropped before anything parsed it.
func TestMessageMatcherPrefilterIsOnlyANecessaryCondition(t *testing.T) {
	line := rawPost("post1", 100, "hello")
	for _, query := range []string{`^hello`, `hello$`, `^hello$`} {
		matcher, err := newMessageMatcher(query, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if matcher.prefilter != nil && !matcher.prefilter(line) {
			t.Errorf("the prefilter dropped a line whose message matches %q", query)
		}
		if !matcher.matches([]byte("hello")) {
			t.Errorf("the message should match %q", query)
		}
	}
	// A pattern whose body reaches a character JSON escapes.
	escaping, err := newMessageMatcher(`hel.*<x>`, true, true)
	if err != nil {
		t.Fatal(err)
	}
	escaped := rawPost("post2", 100, "hello <x>")
	if escaping.prefilter != nil && !escaping.prefilter(escaped) {
		t.Error("the prefilter dropped a line whose message holds an escaped character")
	}
	if !escaping.matches([]byte("hello <x>")) {
		t.Error("the decoded message should match")
	}
}

// TestArchiveWritePostsKeepsPostsSharingTheMark covers two posts written in
// the same millisecond. The mark cannot tell them apart, so the ids at it are
// carried in the state, and without them the second post is skipped for good.
func TestArchiveWritePostsKeepsPostsSharingTheMark(t *testing.T) {
	store, err := archive.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	channel := &archiveChannel{ID: "channel1", Name: "general-chat", TeamName: "example-team"}

	written, err := archiveWritePosts(store, channel, map[string]json.RawMessage{
		"post1": rawPost("post1", 100, "first"),
	}, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if written.newestCreateAt != 100 {
		t.Fatalf("expected the mark at the newest post, got %d", written.newestCreateAt)
	}

	// A second post written in the same millisecond, read on the next sync.
	// The mark cannot tell it from the first, so the end of the file must.
	written, err = archiveWritePosts(store, channel, map[string]json.RawMessage{
		"post1": rawPost("post1", 100, "first"),
		"post2": rawPost("post2", 100, "same millisecond"),
	}, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.fresh) != 1 || written.fresh[0].header.ID != "post2" {
		t.Fatalf("expected post2 to be archived and post1 not, got %+v", written.fresh)
	}

	var ids []string
	if err := store.ScanPosts("example-team", "general-chat", func(line []byte) bool {
		header := &postHeader{}
		if err := json.Unmarshal(line, header); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, header.ID)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ids) != "[post1 post2]" {
		t.Fatalf("expected both posts once each, got %v", ids)
	}
}

// An archive whose state says only how far it got, which is every archive
// written before a sync read the end of the file to tell posts at the mark
// apart. The boundary post must not be stored a second time.
func TestMigrationNoBoundaryIds(t *testing.T) {
	store, _ := archive.Create(t.TempDir())
	channel := &archiveChannel{ID: "c1", Name: "general-chat", TeamName: "example-team"}
	if _, err := archiveWritePosts(store, channel, map[string]json.RawMessage{
		"post1": rawPost("post1", 100, "first"),
	}, 0, true); err != nil {
		t.Fatal(err)
	}
	// Next sync: mark is 100, state has no ids (old archive), server hands
	// back the same post because paging reaches it.
	written, err := archiveWritePosts(store, channel, map[string]json.RawMessage{
		"post1": rawPost("post1", 100, "first"),
	}, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.fresh) != 0 {
		t.Fatalf("post1 was archived a second time: %+v", written.fresh)
	}
	count := 0
	_ = store.ScanPosts("example-team", "general-chat", func(line []byte) bool { count++; return true })
	if count != 1 {
		t.Fatalf("expected 1 post on disk, got %d", count)
	}
}

func TestArchiveIsExcluded(t *testing.T) {
	channel := &archiveChannel{ID: "abcdefghijklmnopqrstuvwxyz", Name: "noisy-alerts", TeamName: "example-team"}
	cases := []struct {
		pattern  string
		expected bool
	}{
		{"abcdefghijklmnopqrstuvwxyz", true},
		{"noisy-alerts", true},
		{"noisy-", true},
		{"alerts", true},
		{"bot-spam", false},
		{"", false},
	}
	for _, testCase := range cases {
		if actual := archiveIsExcluded(channel, []string{testCase.pattern}); actual != testCase.expected {
			t.Errorf("archiveIsExcluded(%q) = %v, expected %v", testCase.pattern, actual, testCase.expected)
		}
	}
	if archiveIsExcluded(channel, nil) {
		t.Error("no patterns must exclude nothing")
	}
}

func TestArchiveChangeExclusions(t *testing.T) {
	patterns := archiveChangeExclusions(nil, []string{"bot-spam", "noisy-alerts"}, false)
	if fmt.Sprint(patterns) != "[bot-spam noisy-alerts]" {
		t.Fatalf("expected both added in order, got %v", patterns)
	}
	// Adding again changes nothing.
	patterns = archiveChangeExclusions(patterns, []string{"bot-spam"}, false)
	if len(patterns) != 2 {
		t.Errorf("a repeat should not be added twice, got %v", patterns)
	}
	patterns = archiveChangeExclusions(patterns, []string{"bot-spam"}, true)
	if fmt.Sprint(patterns) != "[noisy-alerts]" {
		t.Errorf("expected bot-spam removed, got %v", patterns)
	}
}

func TestArchiveWithoutExcluded(t *testing.T) {
	channels := []*archiveChannel{
		{ID: "one", Name: "noisy-alerts"},
		{ID: "two", Name: "general-chat"},
		{ID: "three", Name: "bot-spam"},
	}
	kept, skipped := archiveWithoutExcluded(channels, []string{"noisy-alerts", "bot-spam"})
	if skipped != 2 || len(kept) != 1 || kept[0].Name != "general-chat" {
		t.Fatalf("expected only general-chat kept, got %d skipped and %v", skipped, kept)
	}
	kept, skipped = archiveWithoutExcluded(channels, nil)
	if skipped != 0 || len(kept) != 3 {
		t.Errorf("no patterns must keep everything, got %d skipped", skipped)
	}
}

// TestArchiveWritePostsFailureStopsTheRun covers a disk that cannot be written
// to. That is the archive being broken, not one channel the server declined,
// and a sync must not report it as unreadable and then claim success.
func TestArchiveWritePostsFailureStopsTheRun(t *testing.T) {
	directory := t.TempDir()
	store, err := archive.Create(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveState(map[string]*archive.ChannelState{}); err != nil {
		t.Fatal(err)
	}
	// Nothing further can be written under here.
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Skipf("cannot make the directory read-only: %v", err)
	}
	defer func() { _ = os.Chmod(directory, 0o700) }()

	channel := &archiveChannel{ID: "channel1", Name: "general-chat", TeamName: "example-team"}
	work := &archiveChannelWork{channel: channel}
	outcome := archiveSyncChannelWrite(store, work, map[string]json.RawMessage{
		"post1": rawPost("post1", 100, "first"),
	})
	if outcome.writeError == nil {
		t.Fatal("a write into a read-only archive should have failed")
	}
	if outcome.err != nil {
		t.Errorf("a write failure is not the server declining to read: %v", outcome.err)
	}
	counts := &archiveSyncCounts{}
	if err := archiveRecordChannel(store, map[string]*archive.ChannelState{}, outcome, counts); err == nil {
		t.Error("recording a write failure should have returned an error, not counted it unreadable")
	}
	if counts.unreadableCount != 0 {
		t.Errorf("a write failure must not be counted as unreadable, got %d", counts.unreadableCount)
	}
}
