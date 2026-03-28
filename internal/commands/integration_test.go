package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ziyan/mm/internal/printer"
)

var integrationServerURL string

func TestMain(m *testing.M) {
	integrationServerURL = os.Getenv("MM_SERVER_URL")

	if integrationServerURL != "" {
		configDir, err := os.MkdirTemp("", "mm-integration-config-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create config dir: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = os.RemoveAll(configDir) }()
		_ = os.Setenv("XDG_CONFIG_HOME", configDir)

		if err := waitForServer(integrationServerURL, 120*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "mattermost not ready: %v\n", err)
			os.Exit(1)
		}

		if err := setupMattermost(); err != nil {
			fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

func skipIntegration(t *testing.T) {
	t.Helper()
	if integrationServerURL == "" {
		t.Skip("skipping: MM_SERVER_URL not set")
	}
}

// runCommand executes a CLI command in-process and returns the captured stdout.
func runCommand(args ...string) (string, error) {
	var buf bytes.Buffer
	printer.Stdout = &buf
	// Reset persistent flags to prevent state bleed between calls
	_ = rootCommand.PersistentFlags().Set("json", "false")
	_ = rootCommand.PersistentFlags().Set("log-level", "WARNING")
	_ = rootCommand.PersistentFlags().Set("token", "")
	_ = rootCommand.PersistentFlags().Set("server", "")
	_ = rootCommand.PersistentFlags().Set("team", "")
	rootCommand.SetArgs(args)
	err := rootCommand.Execute()
	return buf.String(), err
}

// runCommandJSON executes a CLI command with --json and parses the output.
func runCommandJSON(args ...string) (interface{}, string, error) {
	output, err := runCommand(append([]string{"--json"}, args...)...)
	if err != nil {
		return nil, output, err
	}
	var result interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		return nil, output, fmt.Errorf("invalid JSON output: %w\nraw: %s", err, output)
	}
	return result, output, nil
}

func waitForServer(url string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	endpoint := url + "/api/v4/system/ping"
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s", url)
		default:
		}

		response, err := http.Get(endpoint)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func setupMattermost() error {
	// Create admin user (first user becomes admin)
	user := map[string]string{
		"email":    "admin@test.local",
		"username": "admin",
		"password": "Admin1234!",
	}
	body, _ := json.Marshal(user)
	response, err := http.Post(integrationServerURL+"/api/v4/users", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("unexpected status creating user: %d", response.StatusCode)
	}

	// Login to get session token
	login := map[string]string{
		"login_id": "admin",
		"password": "Admin1234!",
	}
	body, _ = json.Marshal(login)
	response, err = http.Post(integrationServerURL+"/api/v4/users/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("logging in: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %d", response.StatusCode)
	}
	sessionToken := response.Header.Get("Token")

	var userData map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&userData); err != nil {
		return fmt.Errorf("decoding user data: %w", err)
	}
	userId, _ := userData["id"].(string)

	// Create a personal access token
	tokenPayload := map[string]string{"description": "integration test token"}
	body, _ = json.Marshal(tokenPayload)
	request, _ := http.NewRequest("POST", integrationServerURL+"/api/v4/users/"+userId+"/tokens", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+sessionToken)
	tokenResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("creating access token: %w", err)
	}
	defer func() { _ = tokenResponse.Body.Close() }()
	if tokenResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("creating access token failed: %d", tokenResponse.StatusCode)
	}

	var tokenData map[string]interface{}
	if err := json.NewDecoder(tokenResponse.Body).Decode(&tokenData); err != nil {
		return fmt.Errorf("decoding token data: %w", err)
	}
	accessToken, _ := tokenData["token"].(string)

	// Login via CLI
	output, err := runCommand("auth", "login", "--url", integrationServerURL, "--token", accessToken, "--name", "integration")
	if err != nil {
		return fmt.Errorf("auth login failed: %v\n%s", err, output)
	}

	// Create a team
	teamPayload := map[string]interface{}{
		"name":         "test-team",
		"display_name": "Test Team",
		"type":         "O",
	}
	body, _ = json.Marshal(teamPayload)
	request, _ = http.NewRequest("POST", integrationServerURL+"/api/v4/teams", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	teamCreateResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("creating team: %w", err)
	}
	_ = teamCreateResponse.Body.Close()
	if teamCreateResponse.StatusCode != http.StatusCreated && teamCreateResponse.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("unexpected status creating team: %d", response.StatusCode)
	}

	// Switch to the team via CLI
	output, err = runCommand("team", "switch", "test-team")
	if err != nil {
		return fmt.Errorf("team switch failed: %v\n%s", err, output)
	}

	// Create additional users for DM and group DM tests
	for _, extra := range []map[string]string{
		{"email": "user2@test.local", "username": "testuser2", "password": "User1234!"},
		{"email": "user3@test.local", "username": "testuser3", "password": "User1234!"},
	} {
		if err := createAndAddUserToTeam(accessToken, extra); err != nil {
			return err
		}
	}

	return nil
}

func createAndAddUserToTeam(adminToken string, user map[string]string) error {
	body, _ := json.Marshal(user)
	response, err := http.Post(integrationServerURL+"/api/v4/users", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating user %s: %w", user["username"], err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("unexpected status creating user %s: %d", user["username"], response.StatusCode)
	}

	var userId string
	if response.StatusCode == http.StatusCreated {
		var userData map[string]interface{}
		_ = json.NewDecoder(response.Body).Decode(&userData)
		userId, _ = userData["id"].(string)
	} else {
		request, _ := http.NewRequest("GET", integrationServerURL+"/api/v4/users/username/"+user["username"], nil)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		lookupResponse, err := http.DefaultClient.Do(request)
		if err != nil {
			return fmt.Errorf("looking up user %s: %w", user["username"], err)
		}
		defer func() { _ = lookupResponse.Body.Close() }()
		var userData map[string]interface{}
		_ = json.NewDecoder(lookupResponse.Body).Decode(&userData)
		userId, _ = userData["id"].(string)
	}

	// Get team ID
	request, _ := http.NewRequest("GET", integrationServerURL+"/api/v4/teams/name/test-team", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	teamResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("looking up team: %w", err)
	}
	defer func() { _ = teamResponse.Body.Close() }()
	var teamData map[string]interface{}
	_ = json.NewDecoder(teamResponse.Body).Decode(&teamData)
	teamId, _ := teamData["id"].(string)

	// Add user to team
	memberPayload := map[string]string{"team_id": teamId, "user_id": userId}
	body, _ = json.Marshal(memberPayload)
	request, _ = http.NewRequest("POST", integrationServerURL+"/api/v4/teams/"+teamId+"/members", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+adminToken)
	memberResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("adding user %s to team: %w", user["username"], err)
	}
	_ = memberResponse.Body.Close()

	return nil
}

// --- Integration smoke tests for commands without their own test files ---

func TestIntegrationServerPing(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("server", "ping")
	if err != nil {
		t.Fatalf("server ping failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "reachable") {
		t.Errorf("expected 'reachable' in output, got: %s", output)
	}
}

func TestIntegrationServerInfo(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("server", "info")
	if err != nil {
		t.Fatalf("server info failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Version") {
		t.Errorf("expected 'Version' in output, got: %s", output)
	}
}

func TestIntegrationServerInfoJSON(t *testing.T) {
	skipIntegration(t)
	result, output, err := runCommandJSON("server", "info")
	if err != nil {
		t.Fatalf("server info --json failed: %v\n%s", err, output)
	}
	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	if _, exists := data["Version"]; !exists {
		t.Error("expected 'Version' key in JSON output")
	}
}

func TestIntegrationAuthStatus(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("auth", "status")
	if err != nil {
		t.Fatalf("auth status failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("expected 'admin' in output, got: %s", output)
	}
}

func TestIntegrationAuthList(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("auth", "list")
	if err != nil {
		t.Fatalf("auth list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "integration") {
		t.Errorf("expected 'integration' profile in output, got: %s", output)
	}
}

func TestIntegrationTeamList(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("team", "list")
	if err != nil {
		t.Fatalf("team list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "test-team") {
		t.Errorf("expected 'test-team' in output, got: %s", output)
	}
}

func TestIntegrationTeamInfo(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("team", "info", "test-team")
	if err != nil {
		t.Fatalf("team info failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Test Team") {
		t.Errorf("expected 'Test Team' in output, got: %s", output)
	}
}

func TestIntegrationTeamMembers(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("team", "members", "test-team")
	if err != nil {
		t.Fatalf("team members failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("expected 'admin' in team members, got: %s", output)
	}
}

func TestIntegrationUserMe(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("user", "me")
	if err != nil {
		t.Fatalf("user me failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("expected 'admin' in output, got: %s", output)
	}
}

func TestIntegrationUserStatus(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("user", "status")
	if err != nil {
		t.Fatalf("user status failed: %v", err)
	}
}

func TestIntegrationEmojiList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("emoji", "list")
	if err != nil {
		t.Fatalf("emoji list failed: %v", err)
	}
}

func TestIntegrationPluginList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("plugin", "list")
	if err != nil {
		t.Fatalf("plugin list failed: %v", err)
	}
}

func TestIntegrationDraftList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("draft", "list")
	if err != nil {
		t.Fatalf("draft list failed: %v", err)
	}
}

func TestIntegrationThreadList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("thread", "list")
	if err != nil {
		t.Fatalf("thread list failed: %v", err)
	}
}

func TestIntegrationPreferenceList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("preference", "list")
	if err != nil {
		t.Fatalf("preference list failed: %v", err)
	}
}

func TestIntegrationSavedList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("saved", "list")
	if err != nil {
		t.Fatalf("saved list failed: %v", err)
	}
}

func TestIntegrationWebhookList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("webhook", "list-incoming")
	if err != nil {
		t.Fatalf("webhook list-incoming failed: %v", err)
	}
}

func TestIntegrationBotList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("bot", "list")
	if err != nil {
		t.Fatalf("bot list failed: %v", err)
	}
}

func TestIntegrationGroupList(t *testing.T) {
	skipIntegration(t)
	output, err := runCommand("group", "list")
	if err != nil {
		if strings.Contains(err.Error(), "license") {
			t.Skipf("group list requires enterprise license: %v", err)
		}
		t.Fatalf("group list failed: %v\n%s", err, output)
	}
}

func TestIntegrationSlashList(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("slash", "list")
	if err != nil {
		t.Fatalf("slash list failed: %v", err)
	}
}

func TestIntegrationCompletionBash(t *testing.T) {
	skipIntegration(t)
	// Cobra writes completion scripts directly to os.Stdout (not printer.Stdout),
	// so we just verify the command does not error.
	_, err := runCommand("completion", "bash")
	if err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}
}

func TestIntegrationChannelUnread(t *testing.T) {
	skipIntegration(t)
	_, err := runCommand("channel", "unread")
	if err != nil {
		t.Fatalf("channel unread failed: %v", err)
	}
}
