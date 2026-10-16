package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegrationPostCreateAndList(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("post", "create", "town-square", "Hello from integration test!")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}

	output, err = runCommand("post", "list", "town-square", "-n", "5")
	if err != nil {
		t.Fatalf("post list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Hello from integration test!") {
		t.Errorf("expected message in post list, got: %s", output)
	}
}

func TestIntegrationPostCreateAndListJSON(t *testing.T) {
	skipIntegration(t)
	_, output, err := runCommandJSON("post", "create", "town-square", "JSON test message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}

	result, output, err := runCommandJSON("post", "list", "town-square", "-n", "5")
	if err != nil {
		t.Fatalf("post list --json failed: %v\n%s", err, output)
	}
	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	if _, exists := data["posts"]; !exists {
		t.Error("expected 'posts' key in JSON output")
	}
}

func TestIntegrationPostReplyAndThread(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("post", "create", "town-square", "Thread root message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object for post, got: %T", result)
	}
	postId, _ := post["id"].(string)
	if postId == "" {
		t.Fatalf("could not get post id from: %s", output)
	}

	output, err = runCommand("post", "reply", postId, "Thread reply message")
	if err != nil {
		t.Fatalf("post reply failed: %v\n%s", err, output)
	}

	output, err = runCommand("post", "thread", postId)
	if err != nil {
		t.Fatalf("post thread failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Thread reply message") {
		t.Errorf("expected reply in thread, got: %s", output)
	}
}

func TestIntegrationPostEditAndDelete(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("post", "create", "town-square", "Message to edit")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, _ := result.(map[string]interface{})
	postId, _ := post["id"].(string)

	output, err = runCommand("post", "edit", postId, "Edited message")
	if err != nil {
		t.Fatalf("post edit failed: %v\n%s", err, output)
	}

	output, err = runCommand("post", "delete", postId)
	if err != nil {
		t.Fatalf("post delete failed: %v\n%s", err, output)
	}
}

func TestIntegrationPostPinUnpin(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("post", "create", "town-square", "Pin test message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, _ := result.(map[string]interface{})
	postId, _ := post["id"].(string)

	output, err = runCommand("post", "pin", postId)
	if err != nil {
		t.Fatalf("post pin failed: %v\n%s", err, output)
	}

	output, err = runCommand("post", "unpin", postId)
	if err != nil {
		t.Fatalf("post unpin failed: %v\n%s", err, output)
	}
}

func TestIntegrationPostSearch(t *testing.T) {
	skipIntegration(t)
	uniqueMessage := fmt.Sprintf("unique-search-%d", time.Now().UnixNano())
	output, err := runCommand("post", "create", "town-square", uniqueMessage)
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}

	time.Sleep(2 * time.Second)

	output, err = runCommand("post", "search", uniqueMessage)
	if err != nil {
		t.Fatalf("post search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, uniqueMessage) {
		t.Errorf("expected search to find message, got: %s", output)
	}
}

func TestIntegrationPostReact(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("post", "create", "town-square", "React test message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, _ := result.(map[string]interface{})
	postId, _ := post["id"].(string)

	output, err = runCommand("post", "react", postId, "thumbsup")
	if err != nil {
		t.Fatalf("post react failed: %v\n%s", err, output)
	}

	output, err = runCommand("post", "unreact", postId, "thumbsup")
	if err != nil {
		t.Fatalf("post unreact failed: %v\n%s", err, output)
	}
}

func TestIntegrationSaveAndUnsavePost(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("post", "create", "town-square", "Save test message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, _ := result.(map[string]interface{})
	postId, _ := post["id"].(string)

	output, err = runCommand("saved", "add", postId)
	if err != nil {
		t.Fatalf("saved add failed: %v\n%s", err, output)
	}

	output, err = runCommand("saved", "remove", postId)
	if err != nil {
		t.Fatalf("saved remove failed: %v\n%s", err, output)
	}
}

func TestIntegrationPostReplyWithFile(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("post", "create", "town-square", "thread root for a file reply")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	rootId, _ := result.(map[string]interface{})["id"].(string)

	testFile := filepath.Join(t.TempDir(), "reply-attachment.txt")
	if err := os.WriteFile(testFile, []byte("attached"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, output, err = runCommandJSON("post", "reply", rootId, "-f", testFile)
	if err != nil {
		t.Fatalf("post reply --file failed: %v\n%s", err, output)
	}
	reply, _ := result.(map[string]interface{})
	if reply["root_id"] != rootId {
		t.Errorf("expected root_id %s, got %v", rootId, reply["root_id"])
	}
	if fileIds, _ := reply["file_ids"].([]interface{}); len(fileIds) != 1 {
		t.Errorf("expected one attached file, got %v", reply["file_ids"])
	}

	// Replying to a reply lands in the same thread.
	replyId, _ := reply["id"].(string)
	result, output, err = runCommandJSON("post", "reply", replyId, "second reply")
	if err != nil {
		t.Fatalf("reply to a reply failed: %v\n%s", err, output)
	}
	if second, _ := result.(map[string]interface{}); second["root_id"] != rootId {
		t.Errorf("expected root_id %s, got %v", rootId, second["root_id"])
	}

	if _, err := runCommand("post", "reply", rootId); err == nil {
		t.Error("expected an error with neither a message nor a file")
	}
}

func TestIntegrationFileUploadIntoThread(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("post", "create", "town-square", "thread root for an upload")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	rootId, _ := result.(map[string]interface{})["id"].(string)

	testFile := filepath.Join(t.TempDir(), "thread-upload.txt")
	if err := os.WriteFile(testFile, []byte("attached"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := runCommand("file", "upload", "town-square", testFile, "--root-id", rootId, "-m", "into the thread"); err != nil {
		t.Fatalf("file upload --root-id failed: %v\n%s", err, output)
	}

	_, output, err = runCommandJSON("post", "thread", rootId)
	if err != nil {
		t.Fatalf("post thread failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "into the thread") || !strings.Contains(output, "file_ids") {
		t.Errorf("expected the upload as a reply in the thread, got: %s", output)
	}

	if _, err := runCommand("file", "upload", "off-topic", testFile, "--root-id", rootId); err == nil {
		t.Error("expected an error for a thread in another channel")
	}
}
