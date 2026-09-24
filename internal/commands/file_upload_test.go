package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

// newUploadServer fakes the upload endpoint. It records each uploaded file
// name and answers with an ID derived from that name.
func newUploadServer(t *testing.T, uploadedNames *[]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v4/files" {
			http.NotFound(writer, request)
			return
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		fileHeaders := request.MultipartForm.File["files"]
		if len(fileHeaders) != 1 {
			http.Error(writer, "expected one file", http.StatusBadRequest)
			return
		}
		fileName := fileHeaders[0].Filename
		*uploadedNames = append(*uploadedNames, fileName)
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(model.FileUploadResponse{
			FileInfos: []*model.FileInfo{{Id: "id-" + fileName, Name: fileName}},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestUploadFilesUsesBaseNameAndKeepsOrder(t *testing.T) {
	var uploadedNames []string
	server := newUploadServer(t, &uploadedNames)
	apiClient := model.NewAPIv4Client(server.URL)

	directory := t.TempDir()
	firstPath := filepath.Join(directory, "first.txt")
	secondPath := filepath.Join(directory, "second.png")
	for _, filePath := range []string{firstPath, secondPath} {
		if err := os.WriteFile(filePath, []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	fileIds, err := uploadFiles(context.Background(), apiClient, "channel1", []string{firstPath, secondPath})
	if err != nil {
		t.Fatalf("uploadFiles: %v", err)
	}
	if len(fileIds) != 2 || fileIds[0] != "id-first.txt" || fileIds[1] != "id-second.png" {
		t.Errorf("unexpected file IDs %v", fileIds)
	}
	if len(uploadedNames) != 2 || uploadedNames[0] != "first.txt" || uploadedNames[1] != "second.png" {
		t.Errorf("expected base names on the server, got %v", uploadedNames)
	}
}

func TestUploadFilesMissingFile(t *testing.T) {
	var uploadedNames []string
	server := newUploadServer(t, &uploadedNames)
	apiClient := model.NewAPIv4Client(server.URL)

	_, err := uploadFiles(context.Background(), apiClient, "channel1", []string{filepath.Join(t.TempDir(), "missing.txt")})
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if len(uploadedNames) != 0 {
		t.Errorf("expected no upload, got %v", uploadedNames)
	}
}

func TestUploadFilesNone(t *testing.T) {
	fileIds, err := uploadFiles(context.Background(), model.NewAPIv4Client("http://127.0.0.1:1"), "channel1", nil)
	if err != nil || len(fileIds) != 0 {
		t.Errorf("expected no IDs and no error, got %v, %v", fileIds, err)
	}
}
