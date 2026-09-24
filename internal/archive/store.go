// Package archive reads and writes a local archive of Mattermost posts and
// attachments. The layout is:
//
//	channels.json                  every channel seen, with its team, type and delete_at
//	users.json                     every user seen, so a search can print names offline
//	me.json                        the authenticated user
//	state.json                     per-channel high-water mark, for the next sync
//	posts/<team>/<channel>.jsonl   one post per line, oldest first, as the server sent it
//	files.jsonl                    one record per attachment referenced by an archived post
//	files/<fileId>__<name>         attachment contents
//
// Posts are stored as the raw JSON the server returned rather than as
// re-encoded structs, so a field this version of mm does not know about is
// still there for a later one to read.
package archive

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
)

// The on-disk names below form a compatibility contract with archives that
// earlier tools wrote. The JSON keys therefore keep their original spelling,
// even where the Go field names say more.
const (
	channelsFileName = "channels.json"
	usersFileName    = "users.json"
	meFileName       = "me.json"
	stateFileName    = "state.json"
	excludedFileName = "excluded.json"
	filesFileName    = "files.jsonl"
	postsDirName     = "posts"
	filesDirName     = "files"

	// MaximumNameLength is the cap SafeName puts on one path element. A long
	// team or channel name cannot push a path past what a file system takes.
	MaximumNameLength = 120
)

var unsafeNameCharacters = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// ChannelState is the high-water mark for one channel. A later sync asks the
// server only for posts newer than LastCreateAt.
type ChannelState struct {
	TeamName     string `json:"team"`
	ChannelName  string `json:"name"`
	LastCreateAt int64  `json:"last"`
	IsArchived   bool   `json:"archived"`
}

// ArchivedChannel is a channel as stored in channels.json: everything the
// server sent, plus the team it belongs to.
type ArchivedChannel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	TeamName    string `json:"_team"`
	ChannelType string `json:"type"`
	CreateAt    int64  `json:"create_at"`
	DeleteAt    int64  `json:"delete_at"`
}

// ArchivedFile is one line of files.jsonl.
type ArchivedFile struct {
	FileID      string `json:"file_id"`
	PostID      string `json:"post_id"`
	ChannelName string `json:"channel"`
	TeamName    string `json:"team"`
	CreateAt    int64  `json:"create_at"`
}

// Store is one archive directory. Several goroutines may call its methods at
// once, since a sync reads several channels in parallel. Each of those writes
// its own posts file. The lock below guards what they share, the attachment
// index and the state.
type Store struct {
	directory string
	lock      sync.Mutex
}

// Open prepares an existing archive directory. A command that only reads the
// archive must not create one as a side effect. A mistyped path would then
// look like an empty archive rather than an error.
func Open(directory string) (*Store, error) {
	store, err := newStore(directory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(store.directory)
	if err != nil {
		return nil, fmt.Errorf("archive: opening %s: %w", store.directory, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("archive: %s is not a directory", store.directory)
	}
	if _, err := os.Stat(filepath.Join(store.directory, stateFileName)); err != nil {
		return nil, fmt.Errorf("archive: %s is not an archive directory, it has no %s: %w", store.directory, stateFileName, err)
	}
	return store, nil
}

// Create prepares an archive directory, creating it if it is not there yet.
func Create(directory string) (*Store, error) {
	store, err := newStore(directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(store.directory, 0o755); err != nil {
		return nil, fmt.Errorf("archive: creating %s: %w", store.directory, err)
	}
	return store, nil
}

func newStore(directory string) (*Store, error) {
	if directory == "" {
		return nil, fmt.Errorf("archive: no archive directory given")
	}
	expanded, err := ExpandPath(directory)
	if err != nil {
		return nil, err
	}
	return &Store{directory: expanded}, nil
}

// ExpandPath resolves a leading ~, so a caller may give an archive path exactly
// as a shell takes it.
func ExpandPath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("archive: resolving home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

// Directory is the root of the archive.
func (self *Store) Directory() string {
	return self.directory
}

// PostsDirectory holds one jsonl file per channel, under a directory per team.
func (self *Store) PostsDirectory() string {
	return filepath.Join(self.directory, postsDirName)
}

// FilesDirectory holds downloaded attachments.
func (self *Store) FilesDirectory() string {
	return filepath.Join(self.directory, filesDirName)
}

// PostsPath gives the file holding one channel's posts.
func (self *Store) PostsPath(teamName, channelName string) string {
	return filepath.Join(self.PostsDirectory(), SafeName(teamName), SafeName(channelName)+".jsonl")
}

// FilesPath is the attachment index.
func (self *Store) FilesPath() string {
	return filepath.Join(self.directory, filesFileName)
}

// SafeName turns a team or channel name into something safe to use as a path.
// A name of nothing but dots names the current or parent directory. Another
// tool may have written such a name into the state, so SafeName replaces it
// rather than handing it back.
func SafeName(name string) string {
	safe := unsafeNameCharacters.ReplaceAllString(name, "_")
	if safe == "" || strings.Trim(safe, ".") == "" {
		// One more character than the dots it stands in for, so two names
		// differing only in length do not become the same name.
		safe = strings.Repeat("_", len(safe)+1)
	}
	if len(safe) > MaximumNameLength {
		safe = safe[:MaximumNameLength]
	}
	return safe
}

// LoadState reads the per-channel high-water marks. A missing file is not an
// error: it says only that no sync has run yet.
func (self *Store) LoadState() (map[string]*ChannelState, error) {
	state := map[string]*ChannelState{}
	if err := self.readJson(stateFileName, &state); err != nil {
		if os.IsNotExist(err) {
			return map[string]*ChannelState{}, nil
		}
		return nil, err
	}
	return state, nil
}

// SaveState writes the per-channel high-water marks.
func (self *Store) SaveState(state map[string]*ChannelState) error {
	return self.writeJson(stateFileName, state)
}

// MergeChannels records the channels this sync saw, keeping the ones recorded
// by earlier syncs. A sync narrowed with --only would otherwise drop every
// channel it did not look at from channels.json.
func (self *Store) MergeChannels(channels []json.RawMessage) error {
	var existing []json.RawMessage
	if err := self.readJson(channelsFileName, &existing); err != nil && !os.IsNotExist(err) {
		return err
	}

	order := make([]string, 0, len(existing)+len(channels))
	byId := make(map[string]json.RawMessage, len(existing)+len(channels))
	for _, group := range [][]json.RawMessage{existing, channels} {
		for _, raw := range group {
			header := struct {
				ID string `json:"id"`
			}{}
			if err := json.Unmarshal(raw, &header); err != nil || header.ID == "" {
				continue
			}
			if _, isKnown := byId[header.ID]; !isKnown {
				order = append(order, header.ID)
			}
			byId[header.ID] = raw
		}
	}

	merged := make([]json.RawMessage, 0, len(order))
	for _, channelId := range order {
		merged = append(merged, byId[channelId])
	}
	return self.writeJson(channelsFileName, merged)
}

// LoadChannels reads channels.json, keyed by channel id.
func (self *Store) LoadChannels() (map[string]*ArchivedChannel, error) {
	var channels []*ArchivedChannel
	if err := self.readJson(channelsFileName, &channels); err != nil {
		if os.IsNotExist(err) {
			return map[string]*ArchivedChannel{}, nil
		}
		return nil, err
	}
	byId := make(map[string]*ArchivedChannel, len(channels))
	for _, channel := range channels {
		byId[channel.ID] = channel
	}
	return byId, nil
}

// LoadExcluded reads the channels this archive has been told to leave alone.
// A missing file means none of them.
func (self *Store) LoadExcluded() ([]string, error) {
	var patterns []string
	if err := self.readJson(excludedFileName, &patterns); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return patterns, nil
}

// SaveExcluded records the channels to leave alone from now on.
func (self *Store) SaveExcluded(patterns []string) error {
	return self.writeJson(excludedFileName, patterns)
}

// SaveUsers records the users seen, so a later search can print names without
// going back to the server.
func (self *Store) SaveUsers(users []*model.User) error {
	return self.writeJson(usersFileName, users)
}

// LoadUsernames reads users.json into a user id to username map. A missing file
// yields an empty map, and a search then prints user ids.
func (self *Store) LoadUsernames() (map[string]string, error) {
	var users []*model.User
	if err := self.readJson(usersFileName, &users); err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	usernames := make(map[string]string, len(users))
	for _, user := range users {
		usernames[user.Id] = user.Username
	}
	return usernames, nil
}

// SaveMe records the authenticated user.
func (self *Store) SaveMe(user *model.User) error {
	return self.writeJson(meFileName, user)
}

// AppendPosts adds raw post JSON to one channel's file, oldest first.
//
// A file cut short mid-write ends without a newline. The append then starts
// with one, so the post it adds is not glued onto the partial line in front.
func (self *Store) AppendPosts(teamName, channelName string, lines []json.RawMessage) error {
	path := self.PostsPath(teamName, channelName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("archive: creating %s: %w", filepath.Dir(path), err)
	}
	tail, err := self.ReadTail(teamName, channelName)
	if err != nil {
		return err
	}
	if !tail.EndsWithNewline {
		lines = append([]json.RawMessage{nil}, lines...)
	}

	// The index is only extended if it matched the posts before this append.
	// A channel with no posts yet and no index counts as matching, so a new
	// channel is indexed from its first post.
	postsSize, err := fileSize(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("archive: reading %s: %w", path, err)
	}
	isIndexCurrent := self.isIndexCurrentAt(teamName, channelName, postsSize)
	if os.IsNotExist(err) {
		_, indexErr := os.Stat(self.IndexPath(teamName, channelName))
		isIndexCurrent = os.IsNotExist(indexErr)
	}

	if err := writeLines(path, os.O_APPEND, lines); err != nil {
		return err
	}
	// The posts are written by now, so a failure here only costs speed: a
	// search ignores an index that does not match its posts.
	if isIndexCurrent {
		err = self.appendIndex(teamName, channelName, lines)
	} else {
		// An index that was behind cannot be extended, so it is written again.
		// That happens once, after which appends keep it current.
		err = self.RebuildIndex(teamName, channelName)
	}
	if err != nil {
		log.Warningf("archive: leaving the index of %s/%s out of date: %v", teamName, channelName, err)
	}
	return nil
}

// MergePosts rewrites one channel's file as what is on disk plus additions,
// oldest first, for a sync that reached back before the newest archived post.
// It streams both inputs, since a channel file can run to hundreds of
// megabytes. It writes the new copy beside the old one and renames it over
// the old one. A crash mid-way therefore leaves the previous copy intact,
// rather than a truncated file with a high-water mark that says it is complete.
//
// The caller sorts additions oldest first. Each addition goes in front of the
// first archived post newer than it. A file already out of order is not
// repaired.
func (self *Store) MergePosts(teamName, channelName string, additions []json.RawMessage) error {
	path := self.PostsPath(teamName, channelName)
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

	remaining := additions
	remainingCreateAt := make([]int64, len(additions))
	for index, line := range additions {
		remainingCreateAt[index] = postCreateAt(line)
	}
	writeLine := func(line []byte) error {
		if _, err := writer.Write(line); err != nil {
			return err
		}
		return writer.WriteByte('\n')
	}
	var writeError error
	err = self.ScanPosts(teamName, channelName, func(line []byte) bool {
		createAt := postCreateAt(line)
		for len(remaining) > 0 && remainingCreateAt[0] < createAt {
			if writeError = writeLine(remaining[0]); writeError != nil {
				return false
			}
			remaining = remaining[1:]
			remainingCreateAt = remainingCreateAt[1:]
		}
		writeError = writeLine(line)
		return writeError == nil
	})
	if err != nil {
		return err
	}
	for _, line := range remaining {
		if writeError = writeLine(line); writeError != nil {
			break
		}
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
	// A merge puts posts in the middle of the file, which an append to the
	// index cannot follow, so the index is written again from the posts.
	if err := self.RebuildIndex(teamName, channelName); err != nil {
		log.Warningf("archive: leaving the index of %s/%s out of date: %v", teamName, channelName, err)
	}
	return nil
}

// postCreateAt reads the creation time off one archived post. A line that does
// not parse sorts first, so it stays where it was.
func postCreateAt(line []byte) int64 {
	header := struct {
		CreateAt int64 `json:"create_at"`
	}{}
	_ = json.Unmarshal(line, &header)
	return header.CreateAt
}

func writeLines(path string, mode int, lines []json.RawMessage) error {
	file, err := os.OpenFile(path, mode|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("archive: opening %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	writer := bufio.NewWriter(file)
	for _, line := range lines {
		if _, err := writer.Write(line); err != nil {
			return fmt.Errorf("archive: writing %s: %w", path, err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("archive: writing %s: %w", path, err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("archive: writing %s: %w", path, err)
	}
	return nil
}

// Tail is the end of one channel's file. It holds the posts sharing the
// newest create_at. A sync tells those apart by id, since the mark alone
// cannot. IsComplete is false when the run filled the window. The caller then
// falls back to reading the whole file.
type Tail struct {
	Posts           []json.RawMessage
	NewestCreateAt  int64
	EndsWithNewline bool
	IsComplete      bool
}

// tailWindowSize is how much of the end of a file ReadTail looks at. A run of
// posts sharing one millisecond is a handful at most. This is far more than
// enough, and it bounds the read of a file that may be huge.
const tailWindowSize = 64 * 1024

// ReadTail reads the end of one channel's file. A missing or empty file gives
// an empty tail that is complete, which is what a channel never synced looks
// like.
func (self *Store) ReadTail(teamName, channelName string) (*Tail, error) {
	path := self.PostsPath(teamName, channelName)
	tail := &Tail{EndsWithNewline: true, IsComplete: true}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tail, nil
		}
		return nil, fmt.Errorf("archive: opening %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("archive: reading %s: %w", path, err)
	}
	size := info.Size()
	if size == 0 {
		return tail, nil
	}

	length := int64(tailWindowSize)
	if length > size {
		length = size
	}
	offset := size - length
	window := make([]byte, length)
	if _, err := file.ReadAt(window, offset); err != nil && err != io.EOF {
		return nil, fmt.Errorf("archive: reading %s: %w", path, err)
	}
	tail.EndsWithNewline = window[len(window)-1] == '\n'
	if tail.EndsWithNewline {
		window = window[:len(window)-1]
	}

	lines := bytes.Split(window, []byte("\n"))
	if offset > 0 {
		// The window opened mid-line, so the first one is a fragment.
		lines = lines[1:]
	}
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		if len(line) == 0 {
			continue
		}
		createAt := postCreateAt(line)
		if len(tail.Posts) == 0 {
			tail.NewestCreateAt = createAt
		} else if createAt != tail.NewestCreateAt {
			return tail, nil
		}
		tail.Posts = append(tail.Posts, append(json.RawMessage(nil), line...))
	}
	// Every line in the window shares the one timestamp. Only a read of the
	// whole file can say where the run starts, unless the window was the
	// whole file.
	tail.IsComplete = offset == 0
	return tail, nil
}

// AppendFiles adds attachment records to the attachment index.
func (self *Store) AppendFiles(records []*ArchivedFile) error {
	if len(records) == 0 {
		return nil
	}
	self.lock.Lock()
	defer self.lock.Unlock()
	file, err := os.OpenFile(self.FilesPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("archive: opening %s: %w", self.FilesPath(), err)
	}
	defer func() { _ = file.Close() }()
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("archive: writing %s: %w", self.FilesPath(), err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("archive: writing %s: %w", self.FilesPath(), err)
	}
	return nil
}

// ScanPosts calls visit for every post archived for one channel, oldest first.
// A missing file yields nothing, which is what a channel never synced looks
// like. See ReadPosts for the contract of visit.
func (self *Store) ScanPosts(teamName, channelName string, visit func(line []byte) bool) error {
	path := self.PostsPath(teamName, channelName)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("archive: reading %s: %w", path, err)
	}
	return ReadPosts(path, visit)
}

// ReferencedFileIDs reads every attachment id the archived posts mention.
func (self *Store) ReferencedFileIDs() (map[string]struct{}, error) {
	fileIds := map[string]struct{}{}
	file, err := os.Open(self.FilesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return fileIds, nil
		}
		return nil, fmt.Errorf("archive: opening %s: %w", self.FilesPath(), err)
	}
	defer func() { _ = file.Close() }()
	reader := bufio.NewReaderSize(file, 1<<16)
	for {
		line, err := readLine(reader)
		if len(line) > 0 {
			record := &ArchivedFile{}
			if json.Unmarshal(line, record) == nil && record.FileID != "" {
				fileIds[record.FileID] = struct{}{}
			}
		}
		if err == io.EOF {
			return fileIds, nil
		}
		if err != nil {
			return nil, fmt.Errorf("archive: reading %s: %w", self.FilesPath(), err)
		}
	}
}

// ChannelFile is one archived channel's posts file.
type ChannelFile struct {
	TeamName    string
	ChannelName string
	Path        string
}

// ChannelFiles lists every channel file in the archive, sorted by path.
func (self *Store) ChannelFiles() ([]*ChannelFile, error) {
	var files []*ChannelFile
	root := self.PostsDirectory()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return nil
			}
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		files = append(files, &ChannelFile{
			TeamName:    filepath.Base(filepath.Dir(path)),
			ChannelName: strings.TrimSuffix(filepath.Base(path), ".jsonl"),
			Path:        path,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("archive: walking %s: %w", root, err)
	}
	return files, nil
}

// ReadPosts calls visit for every post in one channel file, oldest first.
// Visit receives the raw line, so a caller may test it cheaply before paying
// for a parse. Returning false stops the walk.
func ReadPosts(path string, visit func(line []byte) bool) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("archive: opening %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	reader := bufio.NewReaderSize(file, 1<<20)
	for {
		line, err := readLine(reader)
		if len(line) > 0 && !visit(line) {
			return nil
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("archive: reading %s: %w", path, err)
		}
	}
}

// readLine reads one line of any length, which bufio.Scanner cannot do: a
// single post carrying a large attachment preview goes past its token limit.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var collected []byte
	for {
		chunk, isPrefix, err := reader.ReadLine()
		collected = append(collected, chunk...)
		if err != nil {
			return collected, err
		}
		if !isPrefix {
			return collected, nil
		}
	}
}

func (self *Store) readJson(name string, target interface{}) error {
	content, err := os.ReadFile(filepath.Join(self.directory, name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(content, target); err != nil {
		return fmt.Errorf("archive: parsing %s: %w", name, err)
	}
	return nil
}

func (self *Store) writeJson(name string, value interface{}) error {
	self.lock.Lock()
	defer self.lock.Unlock()
	path := filepath.Join(self.directory, name)
	content, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("archive: encoding %s: %w", name, err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o644); err != nil {
		return fmt.Errorf("archive: writing %s: %w", path, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("archive: replacing %s: %w", path, err)
	}
	return nil
}
