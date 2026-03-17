package client

import (
	"testing"
)

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
