package archive

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A sync keeps a search index beside each channel's posts. It holds one line
// per post, with only the fields a search or a count reads. A post as the server sends it
// is mostly metadata, such as link previews, reactions, file details and
// properties. On a large archive the message text is a few percent of the
// bytes. Reading the index instead of the posts is what makes a search fast.
//
// The posts stay the source of truth. Each index records the size of the posts
// file it came from. A search ignores an index whose recorded size differs and
// reads the posts instead. A sync cut short, an edit by hand, or an old archive
// can therefore slow a search down. None of them can make it miss a post.
const (
	indexDirName = "index"

	// indexSizeSuffix names the file that records how large the posts file
	// was when its index was last brought up to date.
	indexSizeSuffix = ".size"
)

// indexEntry is one post as the index keeps it. The keys are the server's own,
// so a line of the index parses into anything that parses a post.
type indexEntry struct {
	ID        string `json:"id"`
	CreateAt  int64  `json:"create_at"`
	UserID    string `json:"user_id"`
	ChannelID string `json:"channel_id,omitempty"`
	RootID    string `json:"root_id,omitempty"`
	PostType  string `json:"type,omitempty"`
	Message   string `json:"message"`
}

// IndexDirectory holds the search indexes, laid out like the posts.
func (self *Store) IndexDirectory() string {
	return filepath.Join(self.directory, indexDirName)
}

// IndexPath is one channel's search index.
func (self *Store) IndexPath(teamName, channelName string) string {
	return filepath.Join(self.IndexDirectory(), SafeName(teamName), SafeName(channelName)+".jsonl")
}

func (self *Store) indexSizePath(teamName, channelName string) string {
	return self.IndexPath(teamName, channelName) + indexSizeSuffix
}

// SearchPath is the file a search of one channel should read, and whether it
// is the index rather than the posts. It takes a channel file as ChannelFiles
// lists it, whose names are already the ones on disk.
func (self *Store) SearchPath(channelFile *ChannelFile) (string, bool) {
	indexPath := filepath.Join(self.IndexDirectory(), channelFile.TeamName, channelFile.ChannelName+".jsonl")
	postsSize, err := fileSize(channelFile.Path)
	if err != nil {
		return channelFile.Path, false
	}
	if isIndexFileCurrent(indexPath, postsSize) {
		return indexPath, true
	}
	return channelFile.Path, false
}

// isIndexCurrent reports whether a channel's index describes its posts file as
// it is now.
func (self *Store) isIndexCurrent(teamName, channelName string) bool {
	postsSize, err := fileSize(self.PostsPath(teamName, channelName))
	if err != nil {
		return false
	}
	return self.isIndexCurrentAt(teamName, channelName, postsSize)
}

func (self *Store) isIndexCurrentAt(teamName, channelName string, postsSize int64) bool {
	return isIndexFileCurrent(self.IndexPath(teamName, channelName), postsSize)
}

func isIndexFileCurrent(indexPath string, postsSize int64) bool {
	recorded, err := os.ReadFile(indexPath + indexSizeSuffix)
	if err != nil {
		return false
	}
	recordedSize, err := strconv.ParseInt(strings.TrimSpace(string(recorded)), 10, 64)
	if err != nil || recordedSize != postsSize {
		return false
	}
	_, err = os.Stat(indexPath)
	return err == nil
}

// appendIndex adds the index lines for posts just appended to a channel, and
// records the new size of the posts file.
//
// It is only called when the index was current before the posts were
// appended. Appending to an index that was already behind would record a size
// that matches while the lines in front of these are still missing, and a
// search would trust it.
func (self *Store) appendIndex(teamName, channelName string, lines []json.RawMessage) error {
	path := self.IndexPath(teamName, channelName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("archive: creating %s: %w", filepath.Dir(path), err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("archive: opening %s: %w", path, err)
	}
	writer := bufio.NewWriter(file)
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if err := writeIndexLine(writer, line); err != nil {
			_ = file.Close()
			return fmt.Errorf("archive: writing %s: %w", path, err)
		}
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		return fmt.Errorf("archive: writing %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("archive: writing %s: %w", path, err)
	}
	return self.recordIndexSize(teamName, channelName)
}

// RebuildIndex writes one channel's index from its posts, from scratch.
func (self *Store) RebuildIndex(teamName, channelName string) error {
	postsPath := self.PostsPath(teamName, channelName)
	// The size is read before the posts, so a sync that appends while this
	// runs leaves the index recorded as behind rather than as current.
	postsSize, err := fileSize(postsPath)
	if err != nil {
		return fmt.Errorf("archive: reading %s: %w", postsPath, err)
	}

	path := self.IndexPath(teamName, channelName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("archive: creating %s: %w", filepath.Dir(path), err)
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("archive: opening %s: %w", temporary, err)
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(temporary) // nothing to remove once the rename went through
	}()
	writer := bufio.NewWriterSize(file, 1<<20)

	var writeError error
	err = ReadPosts(postsPath, func(line []byte) bool {
		writeError = writeIndexLine(writer, line)
		return writeError == nil
	})
	if err != nil {
		return err
	}
	if writeError == nil {
		writeError = writer.Flush()
	}
	if writeError != nil {
		return fmt.Errorf("archive: writing %s: %w", temporary, writeError)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("archive: writing %s: %w", temporary, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("archive: replacing %s: %w", path, err)
	}
	return writeFileAtomic(self.indexSizePath(teamName, channelName), []byte(strconv.FormatInt(postsSize, 10)+"\n"))
}

// IsIndexCurrent reports whether a channel's index can stand in for its posts.
func (self *Store) IsIndexCurrent(teamName, channelName string) bool {
	return self.isIndexCurrent(teamName, channelName)
}

func (self *Store) recordIndexSize(teamName, channelName string) error {
	postsSize, err := fileSize(self.PostsPath(teamName, channelName))
	if err != nil {
		return fmt.Errorf("archive: reading the size of %s/%s: %w", teamName, channelName, err)
	}
	return writeFileAtomic(self.indexSizePath(teamName, channelName), []byte(strconv.FormatInt(postsSize, 10)+"\n"))
}

// writeIndexLine writes the index form of one post. A line that does not parse
// is left out, which loses nothing: a search could not have matched it either.
func writeIndexLine(writer io.Writer, line []byte) error {
	entry := &indexEntry{}
	if json.Unmarshal(line, entry) != nil || entry.ID == "" {
		return nil
	}
	encoder := json.NewEncoder(writer)
	// The server does not escape these, and the search's quick test on a raw
	// line assumes the index writes a message the way the server does.
	encoder.SetEscapeHTML(false)
	return encoder.Encode(entry)
}

// CountLines counts the lines in a file without parsing any of them.
func CountLines(path string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("archive: opening %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	buffer := make([]byte, 1<<20)
	var lineCount int64
	var lastByte byte = '\n'
	for {
		readCount, err := file.Read(buffer)
		if readCount > 0 {
			lineCount += int64(bytes.Count(buffer[:readCount], []byte{'\n'}))
			lastByte = buffer[readCount-1]
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("archive: reading %s: %w", path, err)
		}
	}
	// A last line with no newline after it is still a line.
	if lastByte != '\n' {
		lineCount++
	}
	return lineCount, nil
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// writeFileAtomic writes beside the target and renames over it, so a run cut
// short leaves the previous copy rather than half of a new one.
func writeFileAtomic(path string, content []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o644); err != nil {
		return fmt.Errorf("archive: writing %s: %w", path, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("archive: replacing %s: %w", path, err)
	}
	return nil
}
