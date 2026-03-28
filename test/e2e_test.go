//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	serverURL   string
	binary      string
	configDir   string
	coverageDir string
)

func TestMain(m *testing.M) {
	serverURL = os.Getenv("MM_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8065"
	}

	// Build the binary with coverage instrumentation
	root := findProjectRoot()
	binary = filepath.Join(root, "mm-e2e-test")
	build := exec.Command("go", "build", "-mod=vendor", "-cover", "-o", binary, "./command/")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n%s\n", err, output)
		os.Exit(1)
	}
	defer os.Remove(binary)

	// Create isolated config directory
	var err error
	configDir, err = os.MkdirTemp("", "mm-e2e-config-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(configDir)

	// Create coverage data directory
	coverageDir, err = os.MkdirTemp("", "mm-e2e-coverage-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create coverage dir: %v\n", err)
		os.Exit(1)
	}

	// Wait for Mattermost to be ready
	if err := waitForServer(serverURL, 120*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "mattermost not ready: %v\n", err)
		os.Exit(1)
	}

	// Set up initial admin user and get token
	if err := setupAdmin(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup admin: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	// Convert coverage data to text format
	coverageOutput := filepath.Join(root, "coverage")
	os.MkdirAll(coverageOutput, 0755)
	coverageFile := filepath.Join(coverageOutput, "e2e-coverage.out")
	convert := exec.Command("go", "tool", "covdata", "textfmt", "-i="+coverageDir, "-o="+coverageFile)
	if output, err := convert.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to convert coverage data: %v\n%s\n", err, output)
	} else {
		fmt.Fprintf(os.Stderr, "coverage profile written to %s\n", coverageFile)
	}

	os.RemoveAll(coverageDir)
	os.Exit(code)
}

func findProjectRoot() string {
	directory, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	// Fallback: assume we're in e2e/
	return filepath.Dir(directory)
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
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func setupAdmin() error {
	// Create admin user (first user becomes admin)
	user := map[string]string{
		"email":    "admin@test.local",
		"username": "admin",
		"password": "Admin1234!",
	}
	body, _ := json.Marshal(user)
	response, err := http.Post(serverURL+"/api/v4/users", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}
	response.Body.Close()
	// 201 = created, 400 = already exists (both fine)
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("unexpected status creating user: %d", response.StatusCode)
	}

	// Login to get session token
	login := map[string]string{
		"login_id": "admin",
		"password": "Admin1234!",
	}
	body, _ = json.Marshal(login)
	response, err = http.Post(serverURL+"/api/v4/users/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("logging in: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %d", response.StatusCode)
	}
	sessionToken := response.Header.Get("Token")

	// Create a personal access token
	var userData map[string]interface{}
	json.NewDecoder(response.Body).Decode(&userData)
	userId, _ := userData["id"].(string)

	tokenPayload := map[string]string{"description": "e2e test token"}
	body, _ = json.Marshal(tokenPayload)
	request, _ := http.NewRequest("POST", serverURL+"/api/v4/users/"+userId+"/tokens", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+sessionToken)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("creating access token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("creating access token failed: %d", response.StatusCode)
	}

	var tokenData map[string]interface{}
	json.NewDecoder(response.Body).Decode(&tokenData)
	accessToken, _ := tokenData["token"].(string)

	// Use the CLI to login with this token
	output, err := runMM("auth", "login", "--url", serverURL, "--token", accessToken, "--name", "e2e")
	if err != nil {
		return fmt.Errorf("mm auth login failed: %v\n%s", err, output)
	}

	// Create a team
	teamPayload := map[string]interface{}{
		"name":         "test-team",
		"display_name": "Test Team",
		"type":         "O",
	}
	body, _ = json.Marshal(teamPayload)
	request, _ = http.NewRequest("POST", serverURL+"/api/v4/teams", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("creating team: %w", err)
	}
	response.Body.Close()
	// 201 = created, 400 = already exists
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("unexpected status creating team: %d", response.StatusCode)
	}

	// Switch to the team via CLI
	output, err = runMM("team", "switch", "test-team")
	if err != nil {
		return fmt.Errorf("mm team switch failed: %v\n%s", err, output)
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
	response, err := http.Post(serverURL+"/api/v4/users", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating user %s: %w", user["username"], err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("unexpected status creating user %s: %d", user["username"], response.StatusCode)
	}

	// Get user ID (need to look up if already existed)
	var userId string
	if response.StatusCode == http.StatusCreated {
		var userData map[string]interface{}
		json.NewDecoder(response.Body).Decode(&userData)
		userId, _ = userData["id"].(string)
	} else {
		// User exists, look up by username
		request, _ := http.NewRequest("GET", serverURL+"/api/v4/users/username/"+user["username"], nil)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return fmt.Errorf("looking up user %s: %w", user["username"], err)
		}
		defer response.Body.Close()
		var userData map[string]interface{}
		json.NewDecoder(response.Body).Decode(&userData)
		userId, _ = userData["id"].(string)
	}

	// Get team ID
	request, _ := http.NewRequest("GET", serverURL+"/api/v4/teams/name/test-team", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response2, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("looking up team: %w", err)
	}
	defer response2.Body.Close()
	var teamData map[string]interface{}
	json.NewDecoder(response2.Body).Decode(&teamData)
	teamId, _ := teamData["id"].(string)

	// Add user to team
	memberPayload := map[string]string{"team_id": teamId, "user_id": userId}
	body, _ = json.Marshal(memberPayload)
	request, _ = http.NewRequest("POST", serverURL+"/api/v4/teams/"+teamId+"/members", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response3, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("adding user %s to team: %w", user["username"], err)
	}
	response3.Body.Close()

	return nil
}

// runMM executes the mm binary with the given arguments and returns combined output.
func runMM(args ...string) (string, error) {
	command := exec.Command(binary, args...)
	command.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+configDir,
		"GOCOVERDIR="+coverageDir,
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	return output.String(), err
}

// runMMJSON executes the mm binary with --json and parses the output.
func runMMJSON(args ...string) (interface{}, string, error) {
	allArgs := append([]string{"--json"}, args...)
	output, err := runMM(allArgs...)
	if err != nil {
		return nil, output, err
	}
	// Find the first JSON object or array in output (skip non-JSON lines like status messages)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			var result interface{}
			if err := json.Unmarshal([]byte(strings.Join(lines[index:], "\n")), &result); err == nil {
				return result, output, nil
			}
		}
	}
	// Try parsing the entire output
	var result interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, output, fmt.Errorf("failed to parse JSON: %w\noutput: %s", err, output)
	}
	return result, output, nil
}

// --- Tests ---

func TestServerPing(t *testing.T) {
	output, err := runMM("server", "ping")
	if err != nil {
		t.Fatalf("server ping failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "reachable") {
		t.Errorf("expected 'reachable' in output, got: %s", output)
	}
}

func TestServerInfo(t *testing.T) {
	output, err := runMM("server", "info")
	if err != nil {
		t.Fatalf("server info failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Version") {
		t.Errorf("expected 'Version' in output, got: %s", output)
	}
}

func TestServerInfoJSON(t *testing.T) {
	result, output, err := runMMJSON("server", "info")
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

func TestAuthStatus(t *testing.T) {
	output, err := runMM("auth", "status")
	if err != nil {
		t.Fatalf("auth status failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("expected 'admin' in output, got: %s", output)
	}
	if !strings.Contains(output, "e2e") {
		t.Errorf("expected profile name 'e2e' in output, got: %s", output)
	}
}

func TestAuthList(t *testing.T) {
	output, err := runMM("auth", "list")
	if err != nil {
		t.Fatalf("auth list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "e2e") {
		t.Errorf("expected 'e2e' profile in output, got: %s", output)
	}
}

func TestTeamList(t *testing.T) {
	output, err := runMM("team", "list")
	if err != nil {
		t.Fatalf("team list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "test-team") {
		t.Errorf("expected 'test-team' in output, got: %s", output)
	}
}

func TestTeamInfo(t *testing.T) {
	output, err := runMM("team", "info", "test-team")
	if err != nil {
		t.Fatalf("team info failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Test Team") {
		t.Errorf("expected 'Test Team' in output, got: %s", output)
	}
}

func TestUserMe(t *testing.T) {
	output, err := runMM("user", "me")
	if err != nil {
		t.Fatalf("user me failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("expected 'admin' in output, got: %s", output)
	}
}

func TestChannelCreate(t *testing.T) {
	channelName := fmt.Sprintf("e2e-create-%d", time.Now().UnixNano())
	output, err := runMM("channel", "create", channelName, "--display-name", "E2E Test Channel")
	if err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}
	if !strings.Contains(strings.ToLower(output), "created") && !strings.Contains(strings.ToLower(output), channelName) {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestChannelList(t *testing.T) {
	output, err := runMM("channel", "list")
	if err != nil {
		t.Fatalf("channel list failed: %v\n%s", err, output)
	}
	// Should show at least the default channels (town-square, off-topic)
	if !strings.Contains(output, "town-square") {
		t.Errorf("expected 'town-square' in channel list, got: %s", output)
	}
}

func TestChannelJoinLeave(t *testing.T) {
	channelName := fmt.Sprintf("e2e-join-%d", time.Now().UnixNano())
	output, err := runMM("channel", "create", channelName, "--display-name", "E2E Join Channel")
	if err != nil {
		t.Fatalf("channel create failed: %v\n%s", err, output)
	}

	output, err = runMM("channel", "leave", channelName)
	if err != nil {
		t.Fatalf("channel leave failed: %v\n%s", err, output)
	}

	output, err = runMM("channel", "join", channelName)
	if err != nil {
		t.Fatalf("channel join failed: %v\n%s", err, output)
	}
}

func TestPostCreateAndList(t *testing.T) {
	output, err := runMM("post", "create", "town-square", "Hello from e2e test!")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}

	// List posts and verify our message appears
	output, err = runMM("post", "list", "town-square", "-n", "5")
	if err != nil {
		t.Fatalf("post list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Hello from e2e test!") {
		t.Errorf("expected message in post list, got: %s", output)
	}
}

func TestPostCreateAndListJSON(t *testing.T) {
	output, err := runMM("post", "create", "town-square", "JSON test message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}

	result, output, err := runMMJSON("post", "list", "town-square", "-n", "5")
	if err != nil {
		t.Fatalf("post list --json failed: %v\n%s", err, output)
	}
	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	if _, exists := data["posts"]; !exists {
		t.Error("expected 'posts' key in JSON output")
	}
}

func TestPostReplyAndThread(t *testing.T) {
	// Create a root post
	result, output, err := runMMJSON("post", "create", "town-square", "Thread root message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object for post, got: %T", result)
	}
	postId, _ := post["id"].(string)
	if postId == "" {
		t.Fatalf("could not get post id from: %s", output)
	}

	// Reply to the thread
	output, err = runMM("post", "reply", postId, "Thread reply message")
	if err != nil {
		t.Fatalf("post reply failed: %v\n%s", err, output)
	}

	// View the thread
	output, err = runMM("post", "thread", postId)
	if err != nil {
		t.Fatalf("post thread failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Thread reply message") {
		t.Errorf("expected reply in thread, got: %s", output)
	}
}

func TestPostEditAndDelete(t *testing.T) {
	// Create a post
	result, output, err := runMMJSON("post", "create", "town-square", "Message to edit")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, _ := result.(map[string]interface{})
	postId, _ := post["id"].(string)

	// Edit it
	output, err = runMM("post", "edit", postId, "Edited message")
	if err != nil {
		t.Fatalf("post edit failed: %v\n%s", err, output)
	}

	// Delete it
	output, err = runMM("post", "delete", postId)
	if err != nil {
		t.Fatalf("post delete failed: %v\n%s", err, output)
	}
}

func TestPostPinUnpin(t *testing.T) {
	result, output, err := runMMJSON("post", "create", "town-square", "Pin test message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, _ := result.(map[string]interface{})
	postId, _ := post["id"].(string)

	output, err = runMM("post", "pin", postId)
	if err != nil {
		t.Fatalf("post pin failed: %v\n%s", err, output)
	}

	output, err = runMM("post", "unpin", postId)
	if err != nil {
		t.Fatalf("post unpin failed: %v\n%s", err, output)
	}
}

func TestPostSearch(t *testing.T) {
	// Create a post with unique content
	uniqueMessage := fmt.Sprintf("unique-e2e-search-%d", time.Now().UnixNano())
	output, err := runMM("post", "create", "town-square", uniqueMessage)
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}

	// Give Mattermost a moment to index
	time.Sleep(2 * time.Second)

	output, err = runMM("post", "search", uniqueMessage)
	if err != nil {
		t.Fatalf("post search failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, uniqueMessage) {
		t.Errorf("expected search to find message, got: %s", output)
	}
}

func TestChannelInfo(t *testing.T) {
	output, err := runMM("channel", "info", "town-square")
	if err != nil {
		t.Fatalf("channel info failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "town-square") || !strings.Contains(output, "Town Square") {
		t.Errorf("expected channel info, got: %s", output)
	}
}

func TestChannelMembers(t *testing.T) {
	output, err := runMM("channel", "members", "town-square")
	if err != nil {
		t.Fatalf("channel members failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("expected admin in channel members, got: %s", output)
	}
}

func TestTeamMembers(t *testing.T) {
	output, err := runMM("team", "members", "test-team")
	if err != nil {
		t.Fatalf("team members failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("expected admin in team members, got: %s", output)
	}
}

func TestEmojiList(t *testing.T) {
	output, err := runMM("emoji", "list")
	if err != nil {
		t.Fatalf("emoji list failed: %v\n%s", err, output)
	}
	// Empty list is fine, just verify the command works
}

func TestPluginList(t *testing.T) {
	output, err := runMM("plugin", "list")
	if err != nil {
		t.Fatalf("plugin list failed: %v\n%s", err, output)
	}
	// Empty list is fine, just verify the command works
}

func TestDraftList(t *testing.T) {
	output, err := runMM("draft", "list")
	if err != nil {
		t.Fatalf("draft list failed: %v\n%s", err, output)
	}
	// Empty list is fine
}

func TestThreadList(t *testing.T) {
	output, err := runMM("thread", "list")
	if err != nil {
		t.Fatalf("thread list failed: %v\n%s", err, output)
	}
	// May be empty, just verify it works
}

func TestPreferenceList(t *testing.T) {
	output, err := runMM("preference", "list")
	if err != nil {
		t.Fatalf("preference list failed: %v\n%s", err, output)
	}
	// Output depends on server state
}

func TestSavedList(t *testing.T) {
	output, err := runMM("saved", "list")
	if err != nil {
		t.Fatalf("saved list failed: %v\n%s", err, output)
	}
	// Empty list is fine
}

func TestSaveAndUnsavePost(t *testing.T) {
	result, output, err := runMMJSON("post", "create", "town-square", "Save test message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, _ := result.(map[string]interface{})
	postId, _ := post["id"].(string)

	output, err = runMM("saved", "add", postId)
	if err != nil {
		t.Fatalf("saved add failed: %v\n%s", err, output)
	}

	output, err = runMM("saved", "remove", postId)
	if err != nil {
		t.Fatalf("saved remove failed: %v\n%s", err, output)
	}
}

func TestPostReact(t *testing.T) {
	result, output, err := runMMJSON("post", "create", "town-square", "React test message")
	if err != nil {
		t.Fatalf("post create failed: %v\n%s", err, output)
	}
	post, _ := result.(map[string]interface{})
	postId, _ := post["id"].(string)

	output, err = runMM("post", "react", postId, "thumbsup")
	if err != nil {
		t.Fatalf("post react failed: %v\n%s", err, output)
	}

	output, err = runMM("post", "unreact", postId, "thumbsup")
	if err != nil {
		t.Fatalf("post unreact failed: %v\n%s", err, output)
	}
}

func TestUserStatus(t *testing.T) {
	output, err := runMM("user", "status")
	if err != nil {
		t.Fatalf("user status failed: %v\n%s", err, output)
	}
	// Should show some status (online, away, etc.)
}

func TestSlashList(t *testing.T) {
	output, err := runMM("slash", "list")
	if err != nil {
		t.Fatalf("slash list failed: %v\n%s", err, output)
	}
	// Built-in slash commands should be listed
}

func TestWebhookList(t *testing.T) {
	output, err := runMM("webhook", "list-incoming")
	if err != nil {
		t.Fatalf("webhook list-incoming failed: %v\n%s", err, output)
	}
	// Empty list is fine
}

func TestBotList(t *testing.T) {
	output, err := runMM("bot", "list")
	if err != nil {
		t.Fatalf("bot list failed: %v\n%s", err, output)
	}
	// Empty list is fine
}

func TestGroupList(t *testing.T) {
	output, err := runMM("group", "list")
	if err != nil {
		// Groups require an enterprise license
		if strings.Contains(output, "license") {
			t.Skipf("group list requires enterprise license: %s", output)
		}
		t.Fatalf("group list failed: %v\n%s", err, output)
	}
	// Empty list is fine
}

func TestChannelUnread(t *testing.T) {
	output, err := runMM("channel", "unread")
	if err != nil {
		t.Fatalf("channel unread failed: %v\n%s", err, output)
	}
	// May or may not have unread channels
}

func TestCompletionBash(t *testing.T) {
	output, err := runMM("completion", "bash")
	if err != nil {
		t.Fatalf("completion bash failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "bash") && !strings.Contains(output, "completion") {
		t.Errorf("expected bash completion script, got: %s", output[:min(len(output), 200)])
	}
}

// --- DM Tests ---

func TestDMSend(t *testing.T) {
	output, err := runMM("dm", "send", "testuser2", "Hello from DM test!")
	if err != nil {
		t.Fatalf("dm send failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Sent DM") {
		t.Errorf("expected 'Sent DM' in output, got: %s", output)
	}
}

func TestDMSendJSON(t *testing.T) {
	result, output, err := runMMJSON("dm", "send", "testuser2", "JSON DM test message")
	if err != nil {
		t.Fatalf("dm send --json failed: %v\n%s", err, output)
	}
	post, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	message, _ := post["message"].(string)
	if message != "JSON DM test message" {
		t.Errorf("expected message 'JSON DM test message', got: %s", message)
	}
}

func TestDMRead(t *testing.T) {
	// Send a message first
	runMM("dm", "send", "testuser2", "DM read test message")

	output, err := runMM("dm", "read", "testuser2")
	if err != nil {
		t.Fatalf("dm read failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "DM read test message") {
		t.Errorf("expected message in DM history, got: %s", output)
	}
}

func TestDMReadJSON(t *testing.T) {
	result, output, err := runMMJSON("dm", "read", "testuser2")
	if err != nil {
		t.Fatalf("dm read --json failed: %v\n%s", err, output)
	}
	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	if _, exists := data["posts"]; !exists {
		t.Error("expected 'posts' key in JSON output")
	}
}

func TestDMList(t *testing.T) {
	// Ensure we have at least one DM conversation
	runMM("dm", "send", "testuser2", "ensure DM exists")

	output, err := runMM("dm", "list")
	if err != nil {
		t.Fatalf("dm list failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "testuser2") {
		t.Errorf("expected 'testuser2' in DM list, got: %s", output)
	}
}

func TestDMListJSON(t *testing.T) {
	result, output, err := runMMJSON("dm", "list")
	if err != nil {
		t.Fatalf("dm list --json failed: %v\n%s", err, output)
	}
	channels, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected JSON array, got: %T", result)
	}
	if len(channels) == 0 {
		t.Error("expected at least one DM conversation")
	}
}

func TestDMGroup(t *testing.T) {
	output, err := runMM("dm", "group", "testuser2,testuser3", "Hello group!")
	if err != nil {
		t.Fatalf("dm group failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Sent group message") {
		t.Errorf("expected 'Sent group message' in output, got: %s", output)
	}
}

func TestDMGroupJSON(t *testing.T) {
	result, output, err := runMMJSON("dm", "group", "testuser2,testuser3", "JSON group message")
	if err != nil {
		t.Fatalf("dm group --json failed: %v\n%s", err, output)
	}
	post, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", result)
	}
	message, _ := post["message"].(string)
	if message != "JSON group message" {
		t.Errorf("expected message 'JSON group message', got: %s", message)
	}
}

// --- File Tests ---

func TestFileUploadAndDownload(t *testing.T) {
	// Create a temp file to upload
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test-upload.txt")
	testContent := "Hello from e2e file test!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Upload
	output, err := runMM("file", "upload", "town-square", testFile, "-m", "File upload test")
	if err != nil {
		t.Fatalf("file upload failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Uploaded") {
		t.Errorf("expected 'Uploaded' in output, got: %s", output)
	}

	// Get file ID from the post list (find the post with the file)
	result, output, err := runMMJSON("post", "list", "town-square", "-n", "5")
	if err != nil {
		t.Fatalf("post list failed: %v\n%s", err, output)
	}
	data, _ := result.(map[string]interface{})
	posts, _ := data["posts"].(map[string]interface{})
	var fileId string
	for _, postValue := range posts {
		post, _ := postValue.(map[string]interface{})
		fileIds, _ := post["file_ids"].([]interface{})
		if len(fileIds) > 0 {
			fileId, _ = fileIds[0].(string)
			break
		}
	}
	if fileId == "" {
		t.Fatal("could not find uploaded file ID in posts")
	}

	// File info
	output, err = runMM("file", "info", fileId)
	if err != nil {
		t.Fatalf("file info failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "test-upload.txt") {
		t.Errorf("expected filename in file info, got: %s", output)
	}

	// File info JSON
	infoResult, output, err := runMMJSON("file", "info", fileId)
	if err != nil {
		t.Fatalf("file info --json failed: %v\n%s", err, output)
	}
	fileInfo, _ := infoResult.(map[string]interface{})
	if name, _ := fileInfo["name"].(string); name != "test-upload.txt" {
		t.Errorf("expected filename 'test-upload.txt', got: %s", name)
	}

	// Download
	downloadPath := filepath.Join(tempDir, "downloaded.txt")
	output, err = runMM("file", "download", fileId, downloadPath)
	if err != nil {
		t.Fatalf("file download failed: %v\n%s", err, output)
	}
	downloaded, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(downloaded) != testContent {
		t.Errorf("downloaded content = %q, want %q", string(downloaded), testContent)
	}
}

func TestFileUploadMultiple(t *testing.T) {
	tempDir := t.TempDir()
	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(tempDir, "file2.txt")
	os.WriteFile(file1, []byte("content 1"), 0644)
	os.WriteFile(file2, []byte("content 2"), 0644)

	output, err := runMM("file", "upload", "town-square", file1, file2, "-m", "Multi file upload")
	if err != nil {
		t.Fatalf("file upload multiple failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "2 file(s)") {
		t.Errorf("expected '2 file(s)' in output, got: %s", output)
	}
}

func TestFileSearch(t *testing.T) {
	// Upload a file with a unique name to search for
	tempDir := t.TempDir()
	uniqueName := fmt.Sprintf("searchable-%d.txt", time.Now().UnixNano())
	testFile := filepath.Join(tempDir, uniqueName)
	os.WriteFile(testFile, []byte("searchable content"), 0644)

	output, err := runMM("file", "upload", "town-square", testFile, "-m", "Search test file")
	if err != nil {
		t.Fatalf("file upload failed: %v\n%s", err, output)
	}

	// File search requires server-side indexing which may not be available
	time.Sleep(2 * time.Second)

	output, err = runMM("file", "search", uniqueName)
	if err != nil {
		t.Skipf("file search not available: %v\n%s", err, output)
	}
	if !strings.Contains(output, uniqueName) && !strings.Contains(output, "No files found") {
		t.Errorf("unexpected file search output: %s", output)
	}
}
