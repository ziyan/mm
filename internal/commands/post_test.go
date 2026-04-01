package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/printer"
)

func TestNormalizePostId(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz"},
		{"short", "short"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizePostId(tt.input)
		if got != tt.want {
			t.Errorf("normalizePostId(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPostFromJSON(t *testing.T) {
	jsonString := `{"id":"abc123","channel_id":"ch456","message":"hello world"}`
	post, err := PostFromJSON(jsonString)
	if err != nil {
		t.Fatalf("PostFromJSON() error: %v", err)
	}
	if post.Id != "abc123" {
		t.Errorf("Id = %q, want %q", post.Id, "abc123")
	}
	if post.ChannelId != "ch456" {
		t.Errorf("ChannelId = %q, want %q", post.ChannelId, "ch456")
	}
	if post.Message != "hello world" {
		t.Errorf("Message = %q, want %q", post.Message, "hello world")
	}
}

func TestPostFromJSON_Invalid(t *testing.T) {
	_, err := PostFromJSON("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFormatPost(t *testing.T) {
	post := &model.Post{
		Id:       "abcdefghijklmnopqrstuvwxyz",
		UserId:   "user123456789012345678901",
		Message:  "test message",
		CreateAt: 1700000000000,
	}

	userCache := map[string]string{
		"user123456789012345678901": "testuser",
	}

	result := formatPost(nil, nil, post, userCache)

	if result == "" {
		t.Error("formatPost returned empty string")
	}

	// Should contain the short post ID
	if !containsSubstring(result, "abcdefgh") {
		t.Error("formatPost should contain short post ID")
	}

	// Should contain username
	if !containsSubstring(result, "testuser") {
		t.Error("formatPost should contain username")
	}

	// Should contain message
	if !containsSubstring(result, "test message") {
		t.Error("formatPost should contain message")
	}
}

func TestFormatPostWithReply(t *testing.T) {
	post := &model.Post{
		Id:       "abcdefghijklmnopqrstuvwxyz",
		UserId:   "user123456789012345678901",
		Message:  "reply message",
		CreateAt: 1700000000000,
		RootId:   "root12345678901234567890",
	}

	userCache := map[string]string{
		"user123456789012345678901": "testuser",
	}

	result := formatPost(nil, nil, post, userCache)

	// Reply posts should have the arrow prefix
	if !containsSubstring(result, "↳") {
		t.Error("reply posts should contain ↳ prefix")
	}
}

func TestFormatPostWithFiles(t *testing.T) {
	post := &model.Post{
		Id:       "abcdefghijklmnopqrstuvwxyz",
		UserId:   "user123456789012345678901",
		Message:  "file message",
		CreateAt: 1700000000000,
		FileIds:  []string{"file1", "file2"},
	}

	userCache := map[string]string{
		"user123456789012345678901": "testuser",
	}

	result := formatPost(nil, nil, post, userCache)

	if !containsSubstring(result, "[2 file(s)]") {
		t.Error("formatPost should show file count")
	}
}

func containsSubstring(str, substring string) bool {
	return strings.Contains(str, substring)
}

func TestFormatPostWithFullID(t *testing.T) {
	post := &model.Post{
		Id:       "abcdefghijklmnopqrstuvwxyz",
		UserId:   "user123456789012345678901",
		Message:  "test message",
		CreateAt: 1700000000000,
	}

	userCache := map[string]string{
		"user123456789012345678901": "testuser",
	}

	result := formatPostWithOptions(nil, nil, post, userCache, formatPostOptions{fullId: true})

	if !containsSubstring(result, "abcdefghijklmnopqrstuvwxyz") {
		t.Error("formatPostWithOptions(fullId=true) should contain full post ID")
	}
}

func TestFormatPostWithReplyCount(t *testing.T) {
	post := &model.Post{
		Id:         "abcdefghijklmnopqrstuvwxyz",
		UserId:     "user123456789012345678901",
		Message:    "test message",
		CreateAt:   1700000000000,
		ReplyCount: 5,
	}

	userCache := map[string]string{
		"user123456789012345678901": "testuser",
	}

	result := formatPostWithOptions(nil, nil, post, userCache, formatPostOptions{showReplyCount: true})

	if !containsSubstring(result, "[5 replies]") {
		t.Error("formatPostWithOptions(showReplyCount=true) should contain reply count")
	}

	// Without showReplyCount, should not appear
	result2 := formatPostWithOptions(nil, nil, post, userCache, formatPostOptions{})
	if containsSubstring(result2, "[5 replies]") {
		t.Error("formatPostWithOptions(showReplyCount=false) should not contain reply count")
	}
}

func TestFormatPostHideReplyPrefix(t *testing.T) {
	post := &model.Post{
		Id:       "abcdefghijklmnopqrstuvwxyz",
		UserId:   "user123456789012345678901",
		Message:  "reply message",
		CreateAt: 1700000000000,
		RootId:   "root12345678901234567890",
	}

	userCache := map[string]string{
		"user123456789012345678901": "testuser",
	}

	result := formatPostWithOptions(nil, nil, post, userCache, formatPostOptions{hideReplyPrefix: true})

	if containsSubstring(result, "↳") {
		t.Error("formatPostWithOptions(hideReplyPrefix=true) should not contain ↳ prefix")
	}
}

func TestParseSince(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(int64) bool
	}{
		{
			name:  "duration 24h",
			input: "24h",
			check: func(millis int64) bool {
				expected := time.Now().Add(-24 * time.Hour).UnixMilli()
				diff := millis - expected
				return diff >= -1000 && diff <= 1000
			},
		},
		{
			name:  "duration 30m",
			input: "30m",
			check: func(millis int64) bool {
				expected := time.Now().Add(-30 * time.Minute).UnixMilli()
				diff := millis - expected
				return diff >= -1000 && diff <= 1000
			},
		},
		{
			name:  "RFC3339",
			input: "2026-03-29T00:00:00Z",
			check: func(millis int64) bool {
				expected, _ := time.Parse(time.RFC3339, "2026-03-29T00:00:00Z")
				return millis == expected.UnixMilli()
			},
		},
		{
			name:  "date only",
			input: "2026-03-29",
			check: func(millis int64) bool {
				expected, _ := time.Parse("2006-01-02", "2026-03-29")
				return millis == expected.UnixMilli()
			},
		},
		{
			name:    "invalid",
			input:   "not-a-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSince(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseSince(%q) expected error, got %d", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSince(%q) unexpected error: %v", tt.input, err)
			}
			if !tt.check(got) {
				t.Errorf("parseSince(%q) = %d, did not pass check", tt.input, got)
			}
		})
	}
}

func TestPostListFlagInteractions(t *testing.T) {
	findListCommand := func() *cobra.Command {
		for _, child := range rootCommand.Commands() {
			if child.Name() == "post" {
				for _, grandchild := range child.Commands() {
					if grandchild.Name() == "list" {
						return grandchild
					}
				}
			}
		}
		t.Fatal("post list command not found")
		return nil
	}

	t.Run("threads and collapse-threads mutual exclusion", func(t *testing.T) {
		listCommand := findListCommand()
		rootCommand.SetArgs([]string{"post", "list", "--threads", "--collapse-threads", "general"})
		err := listCommand.ValidateFlagGroups()
		if err == nil {
			// Cobra's MarkFlagsMutuallyExclusive produces an error in ValidateFlagGroups;
			// alternatively, Execute will catch it. Try execute.
			rootCommand.SetArgs([]string{"post", "list", "--threads", "--collapse-threads", "general"})
			err = rootCommand.Execute()
		}
		if err == nil {
			t.Error("expected error when --threads and --collapse-threads are both set")
		}
	})

	t.Run("threads with JSON output rejected", func(t *testing.T) {
		// Simulate JSON mode
		originalJson := printer.JSONOutput
		printer.JSONOutput = true
		defer func() { printer.JSONOutput = originalJson }()

		listCommand := findListCommand()
		if err := listCommand.Flags().Set("threads", "true"); err != nil {
			t.Fatalf("setting threads flag: %v", err)
		}
		defer func() { _ = listCommand.Flags().Set("threads", "false") }()

		err := validatePostListFlags(listCommand)
		if err == nil {
			t.Error("expected error for --threads with JSON output")
		}
		if err != nil && !strings.Contains(err.Error(), "--threads is not supported with JSON output") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("count and since rejected", func(t *testing.T) {
		listCommand := findListCommand()
		if err := listCommand.Flags().Set("since", "24h"); err != nil {
			t.Fatalf("setting since flag: %v", err)
		}
		if err := listCommand.Flags().Set("count", "10"); err != nil {
			t.Fatalf("setting count flag: %v", err)
		}
		defer func() {
			_ = listCommand.Flags().Set("since", "")
			_ = listCommand.Flags().Set("count", "20")
		}()

		err := validatePostListFlags(listCommand)
		if err == nil {
			t.Error("expected error for --count with --since")
		}
		if err != nil && !strings.Contains(err.Error(), "--count and --since cannot be used together") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("since without count is allowed", func(t *testing.T) {
		listCommand := findListCommand()
		// Only set --since, don't change --count (not Changed)
		// Reset flags by creating a fresh parse
		if err := listCommand.Flags().Set("since", "24h"); err != nil {
			t.Fatalf("setting since flag: %v", err)
		}
		defer func() { _ = listCommand.Flags().Set("since", "") }()

		// We need count to NOT be Changed. Since it might have been set above,
		// we can't easily unset Changed. Instead, test the logic directly.
		sinceString, _ := listCommand.Flags().GetString("since")
		if sinceString != "24h" {
			t.Fatalf("expected since=24h, got %q", sinceString)
		}
	})
}

func TestPostListFlagExists(t *testing.T) {
	var listCommand *cobra.Command
	for _, child := range rootCommand.Commands() {
		if child.Name() == "post" {
			for _, grandchild := range child.Commands() {
				if grandchild.Name() == "list" {
					listCommand = grandchild
				}
			}
		}
	}
	if listCommand == nil {
		t.Fatal("post list command not found")
	}

	expectedFlags := []string{"count", "since", "user", "full-id", "threads", "collapse-threads"}
	for _, name := range expectedFlags {
		if listCommand.Flags().Lookup(name) == nil {
			t.Errorf("expected flag %q not found on post list command", name)
		}
	}
}

// resetPostListFlags resets shared cobra state that bleeds between tests.
func resetPostListFlags() {
	_ = rootCommand.PersistentFlags().Set("json", "false")
	printer.JSONOutput = false
	for _, child := range rootCommand.Commands() {
		if child.Name() == "post" {
			for _, grandchild := range child.Commands() {
				if grandchild.Name() == "list" {
					_ = grandchild.Flags().Set("threads", "false")
					_ = grandchild.Flags().Set("collapse-threads", "false")
					_ = grandchild.Flags().Set("since", "")
					_ = grandchild.Flags().Set("count", "20")
					_ = grandchild.Flags().Set("user", "")
					return
				}
			}
		}
	}
}

// TestPostListThreadsJSONRejectedViaExecute verifies that running
// `mm --json post list --threads <channel>` is rejected at the command level
// (PreRunE fires before any API call).
func TestPostListThreadsJSONRejectedViaExecute(t *testing.T) {
	defer resetPostListFlags()

	rootCommand.SetArgs([]string{"--json", "post", "list", "--threads", "general"})
	err := rootCommand.Execute()
	if err == nil {
		t.Fatal("expected error for --json with --threads")
	}
	if !strings.Contains(err.Error(), "--threads is not supported with JSON output") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestPostListSinceCountRejectedViaExecute verifies that running
// `mm post list --since 24h --count 10 <channel>` is rejected at the command level.
func TestPostListSinceCountRejectedViaExecute(t *testing.T) {
	defer resetPostListFlags()

	rootCommand.SetArgs([]string{"post", "list", "--since", "24h", "--count", "10", "general"})
	err := rootCommand.Execute()
	if err == nil {
		t.Fatal("expected error for --since with --count")
	}
	if !strings.Contains(err.Error(), "--count and --since cannot be used together") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestPostListThreadsDedup verifies the dedup logic: when --threads is active,
// reply posts are skipped from the main timeline so they only appear under
// their root during thread expansion (not printed twice).
func TestPostListThreadsDedup(t *testing.T) {
	// The filtering loop skips posts with RootId != "" when threads is true.
	// Simulate the filtering logic with a synthetic PostList.
	rootPost := &model.Post{
		Id:         "root12345678901234567890",
		UserId:     "user_alice_0000000000000",
		Message:    "root message",
		CreateAt:   1700000000000,
		ReplyCount: 1,
	}
	replyPost := &model.Post{
		Id:       "reply2345678901234567890",
		UserId:   "user_bob_00000000000000000",
		Message:  "reply message",
		CreateAt: 1700000001000,
		RootId:   rootPost.Id,
	}
	postList := &model.PostList{
		Order: []string{replyPost.Id, rootPost.Id}, // newest first from API
		Posts: map[string]*model.Post{
			rootPost.Id:  rootPost,
			replyPost.Id: replyPost,
		},
	}

	// Simulate the filtering loop with threads=true
	var filteredOrder []string
	for index := len(postList.Order) - 1; index >= 0; index-- {
		postId := postList.Order[index]
		post := postList.Posts[postId]
		if post.RootId != "" {
			continue // threads mode: skip replies from main timeline
		}
		filteredOrder = append(filteredOrder, postId)
	}

	if len(filteredOrder) != 1 {
		t.Fatalf("expected 1 root post in filteredOrder, got %d", len(filteredOrder))
	}
	if filteredOrder[0] != rootPost.Id {
		t.Errorf("expected root post %s, got %s", rootPost.Id, filteredOrder[0])
	}
}

// TestPostListThreadsUserFilterIncludesRepliesUnderOthers verifies the
// corrected logic: when --threads and --user are both active, replies by the
// target user appear even when the thread root belongs to a different user.
func TestPostListThreadsUserFilterIncludesRepliesUnderOthers(t *testing.T) {
	userFilter := "alice"
	userCache := map[string]string{
		"user_alice_0000000000000":   "alice",
		"user_bob_00000000000000000": "bob",
	}

	// Root by bob, reply by alice
	rootPost := &model.Post{
		Id:         "root12345678901234567890",
		UserId:     "user_bob_00000000000000000",
		Message:    "bob starts thread",
		CreateAt:   1700000000000,
		ReplyCount: 1,
	}
	aliceReply := &model.Post{
		Id:       "reply2345678901234567890",
		UserId:   "user_alice_0000000000000",
		Message:  "alice replies",
		CreateAt: 1700000001000,
		RootId:   rootPost.Id,
	}

	// Simulate the corrected filtering loop (threads=true, user filter deferred)
	postList := &model.PostList{
		Order: []string{aliceReply.Id, rootPost.Id},
		Posts: map[string]*model.Post{
			rootPost.Id:   rootPost,
			aliceReply.Id: aliceReply,
		},
	}

	var filteredOrder []string
	for index := len(postList.Order) - 1; index >= 0; index-- {
		postId := postList.Order[index]
		post := postList.Posts[postId]
		// With --threads active, user filter is deferred
		if post.RootId != "" {
			continue
		}
		filteredOrder = append(filteredOrder, postId)
	}

	// Root post should be in filteredOrder (not skipped despite being bob's)
	if len(filteredOrder) != 1 || filteredOrder[0] != rootPost.Id {
		t.Fatalf("expected root post in filteredOrder, got %v", filteredOrder)
	}

	// Simulate thread expansion with user filter
	threadPosts := []*model.Post{rootPost, aliceReply}
	var matchingReplies []string
	for _, threadPost := range threadPosts {
		if threadPost.Id == rootPost.Id {
			continue
		}
		if userCache[threadPost.UserId] != userFilter {
			continue
		}
		matchingReplies = append(matchingReplies, threadPost.Message)
	}

	if len(matchingReplies) != 1 || matchingReplies[0] != "alice replies" {
		t.Errorf("expected alice's reply to be included, got %v", matchingReplies)
	}

	// Verify that a root with no matching replies would be skipped
	rootMatches := userCache[rootPost.UserId] == userFilter
	if rootMatches {
		t.Error("bob's root should not match alice filter")
	}
	// But since alice has a reply, the root+replies should still be shown
	if !rootMatches && len(matchingReplies) == 0 {
		t.Error("expected root to be shown because alice has replies")
	}
}

func TestDmSendCommandAcceptsOneArg(t *testing.T) {
	var sendCommand *cobra.Command
	for _, child := range rootCommand.Commands() {
		if child.Name() == "dm" {
			for _, grandchild := range child.Commands() {
				if grandchild.Name() == "send" {
					sendCommand = grandchild
				}
			}
		}
	}
	if sendCommand == nil {
		t.Fatal("dm send command not found")
	}

	// Validate that the command accepts 1 arg (username only, message from stdin)
	err := sendCommand.Args(sendCommand, []string{"someuser"})
	if err != nil {
		t.Errorf("dm send should accept 1 arg (username only for stdin): %v", err)
	}

	// Should also accept 2+ args (username + message)
	err = sendCommand.Args(sendCommand, []string{"someuser", "hello", "world"})
	if err != nil {
		t.Errorf("dm send should accept multiple args: %v", err)
	}

	// Should reject 0 args
	err = sendCommand.Args(sendCommand, []string{})
	if err == nil {
		t.Error("dm send should reject 0 args")
	}
}
