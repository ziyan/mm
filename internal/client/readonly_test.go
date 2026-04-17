package client

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordingTransport struct {
	calls int
}

func (self *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	self.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    request,
	}, nil
}

func TestIsReadonlySafe(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{"GET", "/api/v4/users/me", true},
		{"HEAD", "/api/v4/users/me", true},
		{"OPTIONS", "/api/v4/users/me", true},
		{"get", "/api/v4/users/me", true},
		{"POST", "/api/v4/users/search", true},
		{"POST", "/api/v4/teams/abc/posts/search", true},
		{"POST", "/api/v4/channels/search", true},
		{"POST", "/api/v4/teams/abc/files/search", true},
		{"POST", "/api/v4/posts", false},
		{"POST", "/api/v4/channels", false},
		{"PUT", "/api/v4/users/me/patch", false},
		{"PATCH", "/api/v4/posts/123/patch", false},
		{"DELETE", "/api/v4/posts/123", false},
	}
	for _, tt := range tests {
		got := isReadonlySafe(tt.method, tt.path)
		if got != tt.want {
			t.Errorf("isReadonlySafe(%q, %q) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

func TestReadonlyTransportAllowsReads(t *testing.T) {
	base := &recordingTransport{}
	transport := &readonlyTransport{profile: "p", base: base}

	allowed := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v4/users/me"},
		{"HEAD", "/api/v4/posts/123"},
		{"OPTIONS", "/api/v4/teams"},
		{"POST", "/api/v4/users/search"},
	}

	for _, tt := range allowed {
		request := httptest.NewRequest(tt.method, "https://mm.example.com"+tt.path, nil)
		response, err := transport.RoundTrip(request)
		if err != nil {
			t.Errorf("RoundTrip(%s %s) error: %v", tt.method, tt.path, err)
			continue
		}
		_ = response.Body.Close()
	}

	if base.calls != len(allowed) {
		t.Errorf("base RoundTripper called %d times, want %d", base.calls, len(allowed))
	}
}

func TestReadonlyTransportBlocksMutations(t *testing.T) {
	base := &recordingTransport{}
	transport := &readonlyTransport{profile: "myprofile", base: base}

	blocked := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v4/posts"},
		{"POST", "/api/v4/channels"},
		{"PUT", "/api/v4/users/me/patch"},
		{"PATCH", "/api/v4/posts/123/patch"},
		{"DELETE", "/api/v4/posts/123"},
	}

	for _, tt := range blocked {
		request := httptest.NewRequest(tt.method, "https://mm.example.com"+tt.path, nil)
		response, err := transport.RoundTrip(request)
		if err == nil {
			t.Errorf("RoundTrip(%s %s) expected error, got nil", tt.method, tt.path)
			if response != nil {
				_ = response.Body.Close()
			}
			continue
		}
		var readonlyError *ReadonlyError
		if !errors.As(err, &readonlyError) {
			t.Errorf("RoundTrip(%s %s) error type = %T, want *ReadonlyError", tt.method, tt.path, err)
			continue
		}
		if readonlyError.Profile != "myprofile" {
			t.Errorf("ReadonlyError.Profile = %q, want %q", readonlyError.Profile, "myprofile")
		}
		if readonlyError.Method != tt.method {
			t.Errorf("ReadonlyError.Method = %q, want %q", readonlyError.Method, tt.method)
		}
		if readonlyError.Path != tt.path {
			t.Errorf("ReadonlyError.Path = %q, want %q", readonlyError.Path, tt.path)
		}
	}

	if base.calls != 0 {
		t.Errorf("base RoundTripper called %d times, want 0", base.calls)
	}
}

func TestReadonlyErrorMessage(t *testing.T) {
	err := &ReadonlyError{Profile: "prod", Method: "POST", Path: "/api/v4/posts"}
	want := `readonly profile "prod": refusing to POST /api/v4/posts`
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}
