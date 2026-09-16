package client

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/ziyan/mm/internal/config"
)

func New() (*model.Client4, *config.ServerProfile, error) {
	configuration, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("client: loading config: %w", err)
	}
	server, err := configuration.ActiveServer()
	if err != nil {
		return nil, nil, err
	}
	serverUrl := server.URL
	if !strings.HasPrefix(serverUrl, "http") {
		serverUrl = "https://" + serverUrl
	}
	serverUrl = strings.TrimRight(serverUrl, "/")
	apiClient := model.NewAPIv4Client(serverUrl)
	apiClient.SetToken(server.Token)
	apiClient.HTTPClient.Transport = newTransport()
	if server.Readonly {
		base := apiClient.HTTPClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		apiClient.HTTPClient.Transport = &readonlyTransport{
			profile: server.Name,
			base:    base,
		}
	}
	return apiClient, server, nil
}

func WebSocketURL(serverUrl string) string {
	url := strings.TrimRight(serverUrl, "/")
	url = strings.Replace(url, "https://", "wss://", 1)
	url = strings.Replace(url, "http://", "ws://", 1)
	if !strings.HasPrefix(url, "ws") {
		url = "wss://" + url
	}
	return url
}

// maximumIdleConnections is how many connections to one server the client
// keeps open. The default of two is enough for a command that makes one
// request at a time. It is not enough for an archive sync reading several
// channels at once, which would otherwise pay a TLS handshake per request.
const maximumIdleConnections = 32

// newTransport is the standard transport, kept warm for several callers at
// once. It is a copy, so nothing here changes the default for anyone else.
func newTransport() http.RoundTripper {
	transport, isStandard := http.DefaultTransport.(*http.Transport)
	if !isStandard {
		return http.DefaultTransport
	}
	cloned := transport.Clone()
	cloned.MaxIdleConns = maximumIdleConnections * 2
	cloned.MaxIdleConnsPerHost = maximumIdleConnections
	return cloned
}
