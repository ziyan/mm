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

func TestNormalizeDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Mujin Team - Quicktron AGVs", "mujin team - quicktron agvs"},
		{"Town Square", "town square"},
		{"  extra   spaces  ", "extra spaces"},
		{"ALLCAPS", "allcaps"},
		{"single", "single"},
		{"hello\tworld", "hello world"},
	}
	for _, test := range tests {
		result := normalizeDisplayName(test.input)
		if result != test.expected {
			t.Errorf("normalizeDisplayName(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestIntegrationChannelResolveByDisplayName(t *testing.T) {
	skipIntegration(t)

	// Create a channel with a multi-word display name
	channelName := fmt.Sprintf("int-dispname-%d", time.Now().UnixNano())
	displayName := "Integration Display Name Test Channel"
	output, err := runCommand("channel", "create", channelName, "--display-name", displayName)
	if err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}

	// Resolve by slug (existing behavior)
	output, err = runCommand("channel", "info", channelName)
	if err != nil {
		t.Fatalf("channel info by slug failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, displayName) {
		t.Errorf("expected display name in output, got: %s", output)
	}

	// Resolve by display name (new behavior)
	output, err = runCommand("channel", "info", displayName)
	if err != nil {
		t.Fatalf("channel info by display name failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, channelName) {
		t.Errorf("expected slug in output when resolved by display name, got: %s", output)
	}

	// Resolve by display name with different casing
	output, err = runCommand("channel", "info", strings.ToLower(displayName))
	if err != nil {
		t.Fatalf("channel info by lowercase display name failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, channelName) {
		t.Errorf("expected slug in output when resolved by lowercase display name, got: %s", output)
	}

	// Resolve default channel "Town Square" by display name
	output, err = runCommand("channel", "info", "Town Square")
	if err != nil {
		t.Fatalf("channel info by display name 'Town Square' failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "town-square") {
		t.Errorf("expected 'town-square' in output, got: %s", output)
	}
}

func TestIntegrationChannelResolveByDisplayNameWithPunctuation(t *testing.T) {
	skipIntegration(t)

	// Create a channel with hyphens, spaces, and mixed case in the display name,
	// mimicking names like "Mujin Team - Quicktron AGVs" that fail search tokenization.
	channelName := fmt.Sprintf("int-punct-%d", time.Now().UnixNano())
	displayName := "Int Team - Quicktron AGVs"
	output, err := runCommand("channel", "create", channelName, "--display-name", displayName)
	if err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}

	// Resolve by exact display name
	output, err = runCommand("channel", "info", displayName)
	if err != nil {
		t.Fatalf("channel info by punctuated display name failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, channelName) {
		t.Errorf("expected slug %q in output, got: %s", channelName, output)
	}

	// Resolve by display name with different casing
	output, err = runCommand("channel", "info", strings.ToLower(displayName))
	if err != nil {
		t.Fatalf("channel info by lowercase punctuated display name failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, channelName) {
		t.Errorf("expected slug %q in output, got: %s", channelName, output)
	}
}
