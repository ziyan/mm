package commands

import (
	"bytes"
	"testing"

	"github.com/ziyan/mm/internal/printer"
)

func TestExecuteHelp(t *testing.T) {
	var stdout bytes.Buffer
	printer.Stdout = &stdout

	rootCommand.SetArgs([]string{"--help"})
	err := rootCommand.Execute()
	if err != nil {
		t.Fatalf("Execute(--help) error: %v", err)
	}

	output := stdout.String() + rootCommand.UsageString()
	if !containsSubstring(output, "mm") {
		t.Error("help output should contain 'mm'")
	}
}

func TestExecuteUnknownCommand(t *testing.T) {
	rootCommand.SetArgs([]string{"nonexistent-command"})
	err := rootCommand.Execute()
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestRootCommandSubcommands(t *testing.T) {
	expectedCommands := []string{
		"auth", "team", "channel", "post", "dm", "user",
		"file", "emoji", "webhook", "bot", "notify",
		"server", "slash", "plugin", "thread", "draft",
		"scheduled", "bookmark", "preference", "saved", "group",
	}

	commandNames := make(map[string]bool)
	for _, child := range rootCommand.Commands() {
		commandNames[child.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !commandNames[expected] {
			t.Errorf("expected command %q not found in root command", expected)
		}
	}
}

func TestJSONFlag(t *testing.T) {
	printer.JSONOutput = false
	rootCommand.SetArgs([]string{"--json", "--help"})
	_ = rootCommand.Execute()

	// PersistentPreRun should have set JSONOutput
	// We can't easily test this without executing a real command,
	// but we can at least verify the flag exists
	flag := rootCommand.PersistentFlags().Lookup("json")
	if flag == nil {
		t.Error("json flag not found")
	}
}

func TestLogLevelFlag(t *testing.T) {
	flag := rootCommand.PersistentFlags().Lookup("log-level")
	if flag == nil {
		t.Fatal("log-level flag not found")
	}
	if flag.Shorthand != "l" {
		t.Errorf("log-level shorthand = %q, want %q", flag.Shorthand, "l")
	}
	if flag.DefValue != "WARNING" {
		t.Errorf("log-level default = %q, want %q", flag.DefValue, "WARNING")
	}
}

func TestTeamFlag(t *testing.T) {
	flag := rootCommand.PersistentFlags().Lookup("team")
	if flag == nil {
		t.Fatal("team flag not found")
	}
	if flag.Shorthand != "T" {
		t.Errorf("team shorthand = %q, want %q", flag.Shorthand, "T")
	}
}
