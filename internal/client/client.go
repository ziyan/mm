package client

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/ziyan/mm/internal/config"
)

func New() (*model.Client4, *config.ServerProfile, error) {
	configuration, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}
	server, err := configuration.ActiveServer()
	if err != nil {
		return nil, nil, err
	}
	serverURL := server.URL
	if !strings.HasPrefix(serverURL, "http") {
		serverURL = "https://" + serverURL
	}
	serverURL = strings.TrimRight(serverURL, "/")
	apiClient := model.NewAPIv4Client(serverURL)
	apiClient.SetToken(server.Token)
	return apiClient, server, nil
}

func WebSocketUrl(serverURL string) string {
	url := strings.TrimRight(serverURL, "/")
	url = strings.Replace(url, "https://", "wss://", 1)
	url = strings.Replace(url, "http://", "ws://", 1)
	if !strings.HasPrefix(url, "ws") {
		url = "wss://" + url
	}
	return url
}
