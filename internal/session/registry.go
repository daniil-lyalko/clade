package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Registry struct {
	baseDir string
}

func NewRegistry(baseDir string) *Registry {
	return &Registry{baseDir: baseDir}
}

func (r *Registry) sessionsDir() string {
	return filepath.Join(r.baseDir, "sessions")
}

func (r *Registry) archiveDir() string {
	return filepath.Join(r.baseDir, "sessions", "archive")
}

func (r *Registry) sessionPath(sessionID string) string {
	safe := sanitizeID(sessionID)
	return filepath.Join(r.sessionsDir(), safe+".json")
}

func (r *Registry) Save(sess *Session) error {
	if err := os.MkdirAll(r.sessionsDir(), 0755); err != nil {
		return fmt.Errorf("failed to create sessions dir: %w", err)
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}
	path := r.sessionPath(sess.SessionID)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}
	return nil
}

func (r *Registry) Get(sessionID string) (*Session, error) {
	path := r.sessionPath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session %q not found: %w", sessionID, err)
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("failed to parse session %q: %w", sessionID, err)
	}
	return &sess, nil
}

func (r *Registry) List() ([]*Session, error) {
	entries, err := os.ReadDir(r.sessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sessions dir: %w", err)
	}
	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(r.sessionsDir(), entry.Name()))
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		sessions = append(sessions, &sess)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActive.After(sessions[j].LastActive)
	})
	return sessions, nil
}

func (r *Registry) Archive(sessionID string) error {
	src := r.sessionPath(sessionID)
	if err := os.MkdirAll(r.archiveDir(), 0755); err != nil {
		return fmt.Errorf("failed to create archive dir: %w", err)
	}
	dst := filepath.Join(r.archiveDir(), sanitizeID(sessionID)+".json")
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("failed to archive session %q: %w", sessionID, err)
	}
	return nil
}

func (r *Registry) ArchiveStale() (int, error) {
	sessions, err := r.List()
	if err != nil {
		return 0, err
	}
	archived := 0
	for _, sess := range sessions {
		if sess.IsArchivable() {
			if err := r.Archive(sess.SessionID); err != nil {
				continue
			}
			archived++
		}
	}
	return archived, nil
}

func (r *Registry) Delete(sessionID string) error {
	path := r.sessionPath(sessionID)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete session %q: %w", sessionID, err)
	}
	return nil
}

func (r *Registry) DropbagPath(sessionID string) string {
	safe := sanitizeID(sessionID)
	return filepath.Join(r.sessionsDir(), safe+".md")
}

func sanitizeID(id string) string {
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "\\", "_")
	id = strings.ReplaceAll(id, "\x00", "")
	return id
}
