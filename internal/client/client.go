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
