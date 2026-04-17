package commands

import (
	"testing"

	"github.com/ziyan/mm/internal/config"
)

func TestParseOnOff(t *testing.T) {
	truthy := []string{"on", "ON", "true", "True", "yes", "1", "enable", "enabled"}
	falsy := []string{"off", "OFF", "false", "no", "0", "disable", "disabled"}
	invalid := []string{"", "maybe", "2", "yess"}

	for _, value := range truthy {
		got, err := parseOnOff(value)
		if err != nil {
			t.Errorf("parseOnOff(%q) error: %v", value, err)
			continue
		}
		if !got {
			t.Errorf("parseOnOff(%q) = false, want true", value)
		}
	}

	for _, value := range falsy {
		got, err := parseOnOff(value)
		if err != nil {
			t.Errorf("parseOnOff(%q) error: %v", value, err)
			continue
		}
		if got {
			t.Errorf("parseOnOff(%q) = true, want false", value)
		}
	}

	for _, value := range invalid {
		if _, err := parseOnOff(value); err == nil {
			t.Errorf("parseOnOff(%q) expected error", value)
		}
	}
}

func TestAuthSetReadonlyTogglesProfile(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)

	configuration := &config.Config{Profiles: map[string]config.ServerProfile{}}
	configuration.SetProfile("test", config.ServerProfile{
		URL:   "https://mm.example.com",
		Token: "tok",
	})
	if err := configuration.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := runCommand("auth", "set-readonly", "test", "on"); err != nil {
		t.Fatalf("set-readonly on: %v", err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.Profiles["test"].Readonly {
		t.Fatalf("expected Readonly=true after set-readonly on")
	}

	if _, err := runCommand("auth", "set-readonly", "test", "off"); err != nil {
		t.Fatalf("set-readonly off: %v", err)
	}
	loaded, err = config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Profiles["test"].Readonly {
		t.Fatalf("expected Readonly=false after set-readonly off")
	}

	if _, err := runCommand("auth", "set-readonly", "missing", "on"); err == nil {
		t.Fatalf("expected error for missing profile")
	}

	if _, err := runCommand("auth", "set-readonly", "test", "maybe"); err == nil {
		t.Fatalf("expected error for invalid value")
	}
}
