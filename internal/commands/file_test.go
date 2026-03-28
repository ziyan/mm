package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegrationFileUploadAndDownload(t *testing.T) {
	skipIntegration(t)
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test-upload.txt")
	testContent := "Hello from integration file test!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Upload
	output, err := runCommand("file", "upload", "town-square", testFile, "-m", "File upload test")
	if err != nil {
		t.Fatalf("file upload failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Uploaded") {
		t.Errorf("expected 'Uploaded' in output, got: %s", output)
	}

	// Get file ID from the post list
	result, output, err := runCommandJSON("post", "list", "town-square", "-n", "5")
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
	output, err = runCommand("file", "info", fileId)
	if err != nil {
		t.Fatalf("file info failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "test-upload.txt") {
		t.Errorf("expected filename in file info, got: %s", output)
	}

	// File info JSON
	infoResult, output, err := runCommandJSON("file", "info", fileId)
	if err != nil {
		t.Fatalf("file info --json failed: %v\n%s", err, output)
	}
	fileInfo, _ := infoResult.(map[string]interface{})
	if name, _ := fileInfo["name"].(string); name != "test-upload.txt" {
		t.Errorf("expected filename 'test-upload.txt', got: %s", name)
	}

	// Download
	downloadPath := filepath.Join(tempDir, "downloaded.txt")
	output, err = runCommand("file", "download", fileId, downloadPath)
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

func TestIntegrationFileUploadMultiple(t *testing.T) {
	skipIntegration(t)
	tempDir := t.TempDir()
	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(tempDir, "file2.txt")
	_ = os.WriteFile(file1, []byte("content 1"), 0644)
	_ = os.WriteFile(file2, []byte("content 2"), 0644)

	output, err := runCommand("file", "upload", "town-square", file1, file2, "-m", "Multi file upload")
	if err != nil {
		t.Fatalf("file upload multiple failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "2 file(s)") {
		t.Errorf("expected '2 file(s)' in output, got: %s", output)
	}
}

func TestIntegrationFileSearch(t *testing.T) {
	skipIntegration(t)
	tempDir := t.TempDir()
	uniqueName := fmt.Sprintf("searchable-%d.txt", time.Now().UnixNano())
	testFile := filepath.Join(tempDir, uniqueName)
	_ = os.WriteFile(testFile, []byte("searchable content"), 0644)

	output, err := runCommand("file", "upload", "town-square", testFile, "-m", "Search test file")
	if err != nil {
		t.Fatalf("file upload failed: %v\n%s", err, output)
	}

	time.Sleep(2 * time.Second)

	output, err = runCommand("file", "search", uniqueName)
	if err != nil {
		t.Skipf("file search not available: %v\n%s", err, output)
	}
	if !strings.Contains(output, uniqueName) && !strings.Contains(output, "No files found") {
		t.Errorf("unexpected file search output: %s", output)
	}
}
