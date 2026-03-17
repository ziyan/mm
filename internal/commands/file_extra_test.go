package commands

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

func TestFormatFileInfoRow(t *testing.T) {
	tests := []struct {
		name     string
		fileInfo *model.FileInfo
		wantSize string
	}{
		{
			name:     "bytes",
			fileInfo: &model.FileInfo{Name: "tiny.txt", Size: 500, MimeType: "text/plain", Id: "id1"},
			wantSize: "500 B",
		},
		{
			name:     "kilobytes",
			fileInfo: &model.FileInfo{Name: "small.txt", Size: 5120, MimeType: "text/plain", Id: "id2"},
			wantSize: "5.0 KB",
		},
		{
			name:     "megabytes",
			fileInfo: &model.FileInfo{Name: "big.zip", Size: 5242880, MimeType: "application/zip", Id: "id3"},
			wantSize: "5.0 MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := formatFileInfoRow(tt.fileInfo)
			if len(row) != 4 {
				t.Fatalf("expected 4 columns, got %d", len(row))
			}
			if row[0] != tt.fileInfo.Name {
				t.Errorf("name = %q, want %q", row[0], tt.fileInfo.Name)
			}
			if row[1] != tt.wantSize {
				t.Errorf("size = %q, want %q", row[1], tt.wantSize)
			}
			if row[2] != tt.fileInfo.MimeType {
				t.Errorf("mime = %q, want %q", row[2], tt.fileInfo.MimeType)
			}
			if row[3] != tt.fileInfo.Id {
				t.Errorf("id = %q, want %q", row[3], tt.fileInfo.Id)
			}
		})
	}
}
