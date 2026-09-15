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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	filesFileName    = "files.jsonl"
	postsDirName     = "posts"
	filesDirName     = "files"
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

// Store is one archive directory.
type Store struct {
	directory string
}

// Open prepares an archive directory, creating it if it is not there yet.
func Open(directory string) (*Store, error) {
	if directory == "" {
		return nil, fmt.Errorf("archive: no archive directory given")
	}
	expanded, err := ExpandPath(directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(expanded, 0o755); err != nil {
		return nil, fmt.Errorf("archive: creating %s: %w", expanded, err)
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
func SafeName(name string) string {
	safe := unsafeNameCharacters.ReplaceAllString(name, "_")
	if len(safe) > 120 {
		safe = safe[:120]
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

// LoadMe reads the authenticated user recorded by an earlier sync.
func (self *Store) LoadMe() (*model.User, error) {
	user := &model.User{}
	if err := self.readJson(meFileName, user); err != nil {
		return nil, err
	}
	return user, nil
}

// AppendPosts adds raw post JSON to one channel's file, oldest first.
func (self *Store) AppendPosts(teamName, channelName string, lines []json.RawMessage) error {
	path := self.PostsPath(teamName, channelName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("archive: creating %s: %w", filepath.Dir(path), err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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

// AppendFiles adds attachment records to the attachment index.
func (self *Store) AppendFiles(records []*ArchivedFile) error {
	if len(records) == 0 {
		return nil
	}
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
