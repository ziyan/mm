package commands

import (
	"fmt"
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
