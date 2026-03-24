package session

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
)

// SessionEntry represents one shooting day / transfer session
type SessionEntry struct {
	Name         string `json:"name"`
	Date         string `json:"date"`
	Status       string `json:"status"` // "in_progress" | "complete"
	Files        int    `json:"files"`
	Bytes        int64  `json:"bytes"`
	HashVerified bool   `json:"hash_verified"`
}

// ProjectState holds the full project state, persisted as JSON on the remote
type ProjectState struct {
	Project  string         `json:"project"`
	Created  string         `json:"created"`
	Sessions []SessionEntry `json:"sessions"`
}

// RemotePath returns the path of the JSON state file on the remote host.
// Always stored in: basePath/projectName/DOCS/.lootup-session.json
func RemotePath(basePath, projectName string) string {
	return filepath.Join(basePath, projectName, "DOCS", ".lootup-session.json")
}

// Load reads and parses the JSON state via SFTP.
// Returns an empty ProjectState if the file doesn't exist (new project).
func Load(client *sftp.Client, remotePath string) (*ProjectState, error) {
	f, err := client.Open(remotePath)
	if err != nil {
		// File doesn't exist — new project
		return &ProjectState{}, nil
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}

	var state ProjectState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse session file: %w", err)
	}

	return &state, nil
}

// Save writes the JSON state via SFTP (pretty-printed)
func Save(client *sftp.Client, remotePath string, state *ProjectState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session state: %w", err)
	}

	f, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create session file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}

	return nil
}

// AddSession appends a new SessionEntry, or updates an existing one by name
func (p *ProjectState) AddSession(entry SessionEntry) {
	for i, s := range p.Sessions {
		if s.Name == entry.Name {
			p.Sessions[i] = entry
			return
		}
	}
	p.Sessions = append(p.Sessions, entry)
}

// HasSession returns true if a session name already exists
func (p *ProjectState) HasSession(name string) bool {
	for _, s := range p.Sessions {
		if s.Name == name {
			return true
		}
	}
	return false
}

// NewProjectState creates a fresh ProjectState for a new project
func NewProjectState(projectName string) *ProjectState {
	return &ProjectState{
		Project: projectName,
		Created: time.Now().Format("2006-01-02"),
	}
}
