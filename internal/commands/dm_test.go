package commands

import (
	"strings"
	"testing"
)

func TestIntegrationDMSend(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("dm", "send", "testuser2", "Hello from DM test!")
	if err != nil {
		t.Fatalf("dm send failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Sent DM") {
		t.Errorf("expected 'Sent DM' in output, got: %s", output)
	}
}

func TestIntegrationDMSendJSON(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("dm", "send", "testuser2", "JSON DM test message")
	if err != nil {
		t.Fatalf("dm send --json failed: %v\n%s", err, output)
	}
	post, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	message, _ := post["message"].(string)
	if message != "JSON DM test message" {
		t.Errorf("expected message 'JSON DM test message', got: %s", message)
	}
}

func TestIntegrationDMRead(t *testing.T) {
	skipIntegration(t)
	_, _ = runCommand("dm", "send", "testuser2", "DM read test message")

	output, err := runCommand("dm", "read", "testuser2")
	if err != nil {
		t.Fatalf("dm read failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "DM read test message") {
		t.Errorf("expected message in DM history, got: %s", output)
	}
}

func TestIntegrationDMReadJSON(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("dm", "read", "testuser2")
	if err != nil {
		t.Fatalf("dm read --json failed: %v\n%s", err, output)
	}
	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	if _, exists := data["posts"]; !exists {
		t.Error("expected 'posts' key in JSON output")
	}
}

func TestIntegrationDMList(t *testing.T) {
	skipIntegration(t)
	_, _ = runCommand("dm", "send", "testuser2", "ensure DM exists")

	output, err := runCommand("dm", "list")
	if err != nil {
		t.Fatalf("dm list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "testuser2") {
		t.Errorf("expected 'testuser2' in DM list, got: %s", output)
	}
}

func TestIntegrationDMListJSON(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("dm", "list")
	if err != nil {
		t.Fatalf("dm list --json failed: %v\n%s", err, output)
	}
	channels, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected JSON array, got: %T", result)
	}
	if len(channels) == 0 {
		t.Error("expected at least one DM conversation")
	}
}

func TestIntegrationDMGroup(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("dm", "group", "testuser2,testuser3", "Hello group!")
	if err != nil {
		t.Fatalf("dm group failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Sent group message") {
		t.Errorf("expected 'Sent group message' in output, got: %s", output)
	}
}

func TestIntegrationDMGroupJSON(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("dm", "group", "testuser2,testuser3", "JSON group message")
	if err != nil {
		t.Fatalf("dm group --json failed: %v\n%s", err, output)
	}
	post, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	message, _ := post["message"].(string)
	if message != "JSON group message" {
		t.Errorf("expected message 'JSON group message', got: %s", message)
	}
}
