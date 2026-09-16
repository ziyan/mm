package client

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

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

const (
	// maximumIdleConnections is how many connections to one server the client
	// keeps open. The default of two is enough for a command that makes one
	// request at a time. It is not enough for an archive sync reading several
	// channels at once, which would otherwise pay a TLS handshake per request.
	maximumIdleConnections = 32

	// responseHeaderTimeout is how long to wait for a server to begin
	// answering. It bounds the wait for the response headers only, so a slow
	// download of a large attachment is not cut off part way.
	responseHeaderTimeout = 2 * time.Minute
)

// newTransport is the standard transport, kept warm for several callers at
// once. It is a copy, so nothing here changes the default for anyone else.
//
// It speaks HTTP/1.1. Over HTTP/2 every request to a server shares one
// connection, and a connection that stops answering without closing takes
// every request on it down with it, with no way to notice: the timeout that
// would catch it is not honoured on an HTTP/2 stream, and the setting that
// would ping the connection to check it does nothing in this version of Go.
// An archive sync reading channels in parallel hung that way for half an hour
// with sixteen workers waiting on one dead connection. On HTTP/1.1 a request
// has a connection to itself, the timeout below applies, and one connection
// going quiet costs one request a retry.
func newTransport() http.RoundTripper {
	transport, isStandard := http.DefaultTransport.(*http.Transport)
	if !isStandard {
		return http.DefaultTransport
	}
	cloned := transport.Clone()
	cloned.MaxIdleConns = maximumIdleConnections * 2
	cloned.MaxIdleConnsPerHost = maximumIdleConnections
	cloned.ResponseHeaderTimeout = responseHeaderTimeout
	cloned.ForceAttemptHTTP2 = false
	// Turning off the HTTP/2 handler is not enough on its own. The protocol
	// is settled in the TLS handshake, so a client still offering h2 there
	// gets h2 frames back and cannot read them.
	cloned.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	if cloned.TLSClientConfig == nil {
		cloned.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	cloned.TLSClientConfig.NextProtos = []string{"http/1.1"}
	return cloned
}
