package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const DirName = ".mcp-obsidian-sync"

type FileEntry struct {
	Path         string    `json:"path"`
	ETag         string    `json:"etag,omitempty"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified,omitempty"`
	LocalSize    int64     `json:"local_size"`
	LocalModTime time.Time `json:"local_mod_time,omitempty"`
}

type Status struct {
	LastPull      time.Time            `json:"last_pull,omitempty"`
	LastPush      time.Time            `json:"last_push,omitempty"`
	LastError     string               `json:"last_error,omitempty"`
	LastErrorTime time.Time            `json:"last_error_time,omitempty"`
	Entries       map[string]FileEntry `json:"entries,omitempty"`
}

type Store struct {
	path string
}

func New(vaultPath string) Store {
	return Store{path: filepath.Join(vaultPath, DirName, "state.json")}
}

func (s Store) Path() string {
	return s.path
}

func (s Store) Load() (Status, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Status{Entries: map[string]FileEntry{}}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, err
	}
	if status.Entries == nil {
		status.Entries = map[string]FileEntry{}
	}
	return status, nil
}

func (s Store) Save(status Status) error {
	if status.Entries == nil {
		status.Entries = map[string]FileEntry{}
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(data, '\n'), 0o600)
}

func (s Store) MarkError(err error) {
	status, loadErr := s.Load()
	if loadErr != nil {
		status = Status{Entries: map[string]FileEntry{}}
	}
	status.LastError = err.Error()
	status.LastErrorTime = time.Now().UTC()
	_ = s.Save(status)
}
