package commands

import (
	"encoding/json"
	"fmt"
	"os"
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
	channel := &archiveChannel{ID: "channel1", Name: "backend", TeamName: "engineering"}

	first := map[string]json.RawMessage{
		"post2": rawPost("post2", 200, "second"),
		"post3": rawPost("post3", 300, "third"),
	}
	fresh, err := archiveWritePosts(store, channel, first, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 2 {
		t.Fatalf("expected 2 fresh posts, got %d", len(fresh))
	}

	// The same posts again, as after a crash before SaveState, plus one newer.
	second := map[string]json.RawMessage{
		"post2": rawPost("post2", 200, "second"),
		"post3": rawPost("post3", 300, "third"),
		"post4": rawPost("post4", 400, "fourth"),
	}
	fresh, err = archiveWritePosts(store, channel, second, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 1 || fresh[0].header.ID != "post4" {
		t.Fatalf("expected only post4 to be fresh, got %+v", fresh)
	}

	// A repair reaching back before what is on disk: merged, in order.
	repair := map[string]json.RawMessage{
		"post1": rawPost("post1", 100, "first"),
		"post3": rawPost("post3", 300, "third"),
	}
	fresh, err = archiveWritePosts(store, channel, repair, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 1 || fresh[0].header.ID != "post1" {
		t.Fatalf("expected only post1 to be fresh, got %+v", fresh)
	}

	var ids []string
	err = store.ScanPosts("engineering", "backend", func(line []byte) bool {
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
	if _, err := os.Stat(store.PostsPath("engineering", "backend") + ".tmp"); !os.IsNotExist(err) {
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
	if matcher.canPrefilter {
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
	if !plain.canPrefilter || !plain.matches(line) {
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
	if anchored.canPrefilter {
		t.Fatal("a regular expression must not be tested against the raw line")
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
	channel := &archiveChannel{ID: "channel1", Name: "backend", TeamName: "engineering"}
	batch := map[string]json.RawMessage{
		"post2": rawPost("post2", 200, "second"),
		"post3": rawPost("post3", 300, "third"),
	}
	if _, err := archiveWritePosts(store, channel, batch, 100, false); err != nil {
		t.Fatal(err)
	}
	// The mark was never advanced past 100, so the same read happens again.
	batch["post4"] = rawPost("post4", 400, "fourth")
	fresh, err := archiveWritePosts(store, channel, batch, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 1 || fresh[0].header.ID != "post4" {
		t.Fatalf("expected only post4 to be fresh after a stale mark, got %d posts", len(fresh))
	}
	count := 0
	if err := store.ScanPosts("engineering", "backend", func(line []byte) bool { count++; return true }); err != nil {
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
	if !prefixed.canPrefilter || !prefixed.matches([]byte("a deadlock in the works")) || prefixed.matches([]byte("a lock in")) {
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
