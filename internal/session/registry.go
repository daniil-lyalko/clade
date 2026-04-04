package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// maxIDLength is the maximum allowed length for a session ID.
const maxIDLength = 255

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
	safe, _ := sanitizeID(sessionID)
	return filepath.Join(r.sessionsDir(), safe+".json")
}

// lockPath returns the path used for advisory file locking on the sessions directory.
func (r *Registry) lockPath() string {
	return filepath.Join(r.sessionsDir(), ".lock")
}

// withLock acquires an exclusive flock on the sessions directory lock file,
// runs fn, then releases the lock. This prevents races between concurrent
// hook invocations (session-start, session-stop, session-compact, async).
func (r *Registry) withLock(fn func() error) error {
	if err := os.MkdirAll(r.sessionsDir(), 0755); err != nil {
		return fmt.Errorf("failed to create sessions dir: %w", err)
	}
	f, err := os.OpenFile(r.lockPath(), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire registry lock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func (r *Registry) Save(sess *Session) error {
	return r.withLock(func() error {
		if err := os.MkdirAll(r.sessionsDir(), 0755); err != nil {
			return fmt.Errorf("failed to create sessions dir: %w", err)
		}
		data, err := json.MarshalIndent(sess, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal session: %w", err)
		}
		path := r.sessionPath(sess.SessionID)
		// Atomic write: write to temp file, then rename into place.
		tmp, err := os.CreateTemp(r.sessionsDir(), ".tmp-*.json")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpName := tmp.Name()
		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("failed to close temp file: %w", err)
		}
		if err := os.Chmod(tmpName, 0600); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("failed to set permissions: %w", err)
		}
		if err := os.Rename(tmpName, path); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("failed to rename temp file: %w", err)
		}
		return nil
	})
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
	return r.withLock(func() error {
		src := r.sessionPath(sessionID)
		if err := os.MkdirAll(r.archiveDir(), 0755); err != nil {
			return fmt.Errorf("failed to create archive dir: %w", err)
		}
		safe, _ := sanitizeID(sessionID)
		dst := filepath.Join(r.archiveDir(), safe+".json")
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("failed to archive session %q: %w", sessionID, err)
		}
		return nil
	})
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
	safe, _ := sanitizeID(sessionID)
	return filepath.Join(r.sessionsDir(), safe+".md")
}

// sanitizeID validates and cleans a session ID for safe use as a filename.
// Returns an error for empty strings, path traversal attempts, or IDs exceeding
// the maximum length.
func sanitizeID(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("session ID must not be empty")
	}
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "\\", "_")
	id = strings.ReplaceAll(id, "\x00", "")
	// Reject path traversal sequences after slash replacement
	id = strings.ReplaceAll(id, "..", "_")
	if id == "" {
		return "", fmt.Errorf("session ID is empty after sanitization")
	}
	if len(id) > maxIDLength {
		id = id[:maxIDLength]
	}
	return id, nil
}
