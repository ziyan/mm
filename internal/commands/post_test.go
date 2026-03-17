package commands

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
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
	jsonStr := `{"id":"abc123","channel_id":"ch456","message":"hello world"}`
	post, err := PostFromJSON(jsonStr)
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
	return len(str) >= len(substring) && searchSubstring(str, substring)
}

func searchSubstring(str, substring string) bool {
	for index := 0; index <= len(str)-len(substring); index++ {
		if str[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}
