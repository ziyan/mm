package client

import (
	"fmt"
	"net/http"
	"strings"
)

// ReadonlyError is returned by the readonly transport when a mutating request
// is attempted against a profile marked as readonly.
type ReadonlyError struct {
	Profile string
	Method  string
	Path    string
}

func (self *ReadonlyError) Error() string {
	return fmt.Sprintf("readonly profile %q: refusing to %s %s", self.Profile, self.Method, self.Path)
}

// readonlyTransport blocks any HTTP request that would mutate server state.
//
// GET, HEAD, and OPTIONS are always allowed. POST is allowed only when the
// path ends with "/search", which covers Mattermost's search endpoints
// (users, posts, channels, files, emoji, etc.) — those use POST to carry a
// JSON body but do not mutate state.
type readonlyTransport struct {
	profile string
	base    http.RoundTripper
}

func (self *readonlyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if !isReadonlySafe(request.Method, request.URL.Path) {
		return nil, &ReadonlyError{
			Profile: self.profile,
			Method:  request.Method,
			Path:    request.URL.Path,
		}
	}
	return self.base.RoundTrip(request)
}

func isReadonlySafe(method, path string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	case http.MethodPost:
		return strings.HasSuffix(path, "/search")
	}
	return false
}
