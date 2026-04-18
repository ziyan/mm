package client

import (
	"testing"

	"github.com/ziyan/mm/internal/config"
)

func TestNewInstallsReadonlyTransport(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)

	configuration := &config.Config{Profiles: map[string]config.ServerProfile{}}
	configuration.SetProfile("ro", config.ServerProfile{
		URL:      "https://mm.example.com",
		Token:    "tok",
		Readonly: true,
	})
	configuration.SetProfile("rw", config.ServerProfile{
		URL:   "https://mm.example.com",
		Token: "tok",
	})

	configuration.ActiveProfile = "ro"
	if err := configuration.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	apiClient, server, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !server.Readonly {
		t.Fatalf("server.Readonly = false, want true")
	}
	if _, ok := apiClient.HTTPClient.Transport.(*readonlyTransport); !ok {
		t.Fatalf("HTTPClient.Transport = %T, want *readonlyTransport", apiClient.HTTPClient.Transport)
	}

	configuration.ActiveProfile = "rw"
	if err := configuration.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	apiClient, server, err = New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if server.Readonly {
		t.Fatalf("server.Readonly = true, want false")
	}
	if _, ok := apiClient.HTTPClient.Transport.(*readonlyTransport); ok {
		t.Fatalf("HTTPClient.Transport should not be *readonlyTransport for writable profile")
	}
}

func TestWebSocketUrl(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://mm.example.com", "wss://mm.example.com"},
		{"http://mm.example.com", "ws://mm.example.com"},
		{"https://mm.example.com/", "wss://mm.example.com"},
		{"mm.example.com", "wss://mm.example.com"},
		{"http://localhost:8065", "ws://localhost:8065"},
	}
	for _, tt := range tests {
		got := WebSocketUrl(tt.input)
		if got != tt.want {
			t.Errorf("WebSocketUrl(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
