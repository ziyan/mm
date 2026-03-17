package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLoadSave(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)

	// Load non-existent config
	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(config.Profiles) != 0 {
		t.Fatalf("expected empty profiles, got %d", len(config.Profiles))
	}

	// Set and save
	config.SetProfile("test", ServerProfile{
		URL:      "https://mm.example.com",
		Token:    "tok123",
		Username: "alice",
	})
	if config.ActiveProfile != "test" {
		t.Fatalf("expected active profile 'test', got %q", config.ActiveProfile)
	}

	if err := config.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(directory, "mm", "config.json"))
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	var saved Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if saved.ActiveProfile != "test" {
		t.Fatalf("saved active profile = %q, want 'test'", saved.ActiveProfile)
	}
	profile, ok := saved.Profiles["test"]
	if !ok {
		t.Fatal("profile 'test' not found in saved config")
	}
	if profile.URL != "https://mm.example.com" {
		t.Fatalf("URL = %q, want 'https://mm.example.com'", profile.URL)
	}
	if profile.Token != "tok123" {
		t.Fatalf("Token = %q, want 'tok123'", profile.Token)
	}

	// Reload
	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load() after save error: %v", err)
	}
	if reloaded.ActiveProfile != "test" {
		t.Fatalf("reloaded active = %q, want 'test'", reloaded.ActiveProfile)
	}
	server, err := reloaded.ActiveServer()
	if err != nil {
		t.Fatalf("ActiveServer() error: %v", err)
	}
	if server.Username != "alice" {
		t.Fatalf("Username = %q, want 'alice'", server.Username)
	}
}

func TestActiveServerNoProfile(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)

	config, _ := Load()
	_, err := config.ActiveServer()
	if err == nil {
		t.Fatal("expected error for no active profile")
	}
}

func TestSetProfileMultiple(t *testing.T) {
	config := &Config{Profiles: make(map[string]ServerProfile)}

	config.SetProfile("server1", ServerProfile{URL: "https://s1.com", Token: "t1"})
	config.SetProfile("server2", ServerProfile{URL: "https://s2.com", Token: "t2"})

	// First one becomes active
	if config.ActiveProfile != "server1" {
		t.Fatalf("expected active 'server1', got %q", config.ActiveProfile)
	}
	if len(config.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(config.Profiles))
	}
}
