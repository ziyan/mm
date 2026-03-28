package commands

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIntegrationChannelCreate(t *testing.T) {
	skipIntegration(t)
	channelName := fmt.Sprintf("int-create-%d", time.Now().UnixNano())
	output, err := runCommand("channel", "create", channelName, "--display-name", "Integration Test Channel")
	if err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}
	if !strings.Contains(strings.ToLower(output), "created") && !strings.Contains(strings.ToLower(output), channelName) {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestIntegrationChannelList(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("channel", "list")
	if err != nil {
		t.Fatalf("channel list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "town-square") {
		t.Errorf("expected 'town-square' in channel list, got: %s", output)
	}
}

func TestIntegrationChannelJoinLeave(t *testing.T) {
	skipIntegration(t)
	channelName := fmt.Sprintf("int-join-%d", time.Now().UnixNano())
	output, err := runCommand("channel", "create", channelName, "--display-name", "Integration Join Channel")
	if err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}

	output, err = runCommand("channel", "leave", channelName)
	if err != nil {
		t.Fatalf("channel leave failed: %v\n%s", err, output)
	}

	output, err = runCommand("channel", "join", channelName)
	if err != nil {
		t.Fatalf("channel join failed: %v\n%s", err, output)
	}
}

func TestIntegrationChannelInfo(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("channel", "info", "town-square")
	if err != nil {
		t.Fatalf("channel info failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "town-square") || !strings.Contains(output, "Town Square") {
		t.Errorf("expected channel info, got: %s", output)
	}
}

func TestIntegrationChannelMembers(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("channel", "members", "town-square")
	if err != nil {
		t.Fatalf("channel members failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("expected admin in channel members, got: %s", output)
	}
}
