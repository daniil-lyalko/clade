> **NOTE:** This is the original implementation plan. The actual implementation may differ. See source code for current behavior.

# Clade v0.8: Session Awareness Layer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add cross-session awareness to Clade so every Claude Code session knows what every other session is doing

**Architecture:** Session registry (JSON files in ~/.clade/sessions/), daily inbox (append-only markdown in ~/.clade/inbox/), interactive dashboard+launcher (clade sessions command), non-blocking stop hooks with three-tier triage

**Tech Stack:** Go 1.22, cobra CLI, JSON file I/O, flock for concurrency, JSONL transcript tail-reading

---

## Task Overview

| Task | Component | Files |
|------|-----------|-------|
| 1 | Session types | `internal/session/types.go` |
| 2 | Session registry CRUD | `internal/session/registry.go` |
| 3 | Inbox read/write | `internal/session/inbox.go` |
| 4 | session-start command | `internal/cmd/session_start.go` |
| 5 | session-stop command with triage | `internal/cmd/session_stop.go` |
| 6 | session-stop-async command | `internal/cmd/session_stop_async.go` |
| 7 | session-compact command | `internal/cmd/session_compact.go` |
| 8 | clade sessions interactive dashboard | `internal/cmd/sessions.go` |
| 9 | Update inject-context to scan inbox | `internal/cmd/inject.go` |
| 10 | Update setup command for new hooks | `internal/cmd/setup.go` |
| 11 | Update config paths to ~/.clade/ | `internal/config/config.go` |
| 12 | Migration command | `internal/cmd/migrate_dotclade.go` |
| 13 | Register all new commands in root.go | `internal/cmd/root.go` |
| 14 | /sessions skill | `.claude/commands/sessions.md` |
| 15 | /drop skill update | `.claude/commands/drop.md` |

---

### Task 1: Session Types

**Files:**
- Create: `internal/session/types.go`
- Test: `internal/session/types_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/session/types_test.go`:

```go
package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSessionStatus_String(t *testing.T) {
	assert.Equal(t, "active", string(StatusActive))
	assert.Equal(t, "stopped", string(StatusStopped))
	assert.Equal(t, "stopping", string(StatusStopping))
}

func TestSession_IsStale(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		lastActive time.Time
		want       bool
	}{
		{"just now", now, false},
		{"12 hours ago", now.Add(-12 * time.Hour), false},
		{"25 hours ago", now.Add(-25 * time.Hour), true},
		{"8 days ago", now.Add(-8 * 24 * time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{LastActive: tt.lastActive}
			assert.Equal(t, tt.want, s.IsStale())
		})
	}
}

func TestSession_IsArchivable(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		lastActive time.Time
		want       bool
	}{
		{"1 day ago", now.Add(-24 * time.Hour), false},
		{"6 days ago", now.Add(-6 * 24 * time.Hour), false},
		{"8 days ago", now.Add(-8 * 24 * time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{LastActive: tt.lastActive}
			assert.Equal(t, tt.want, s.IsArchivable())
		})
	}
}

func TestSession_StatusLabel(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		status     SessionStatus
		lastActive time.Time
		want       string
	}{
		{"active", StatusActive, now, "active"},
		{"stopped recent", StatusStopped, now.Add(-1 * time.Hour), "idle 1h"},
		{"stopped stale", StatusStopped, now.Add(-25 * time.Hour), "stale 1d"},
		{"stopping", StatusStopping, now.Add(-3 * time.Minute), "stopping"},
		{"stopping stuck", StatusStopping, now.Add(-6 * time.Minute), "stopped (incomplete)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{Status: tt.status, LastActive: tt.lastActive}
			assert.Equal(t, tt.want, s.StatusLabel())
		})
	}
}

func TestInboxEntryType_IsValid(t *testing.T) {
	assert.True(t, EntryDecision.IsValid())
	assert.True(t, EntryFYI.IsValid())
	assert.True(t, EntryBlocker.IsValid())
	assert.True(t, EntryHandoff.IsValid())
	assert.False(t, InboxEntryType("invalid").IsValid())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/session/...`

Expected: FAIL — package `internal/session` does not exist

- [ ] **Step 3: Write minimal implementation**

Create `internal/session/types.go`:

```go
package session

import (
	"fmt"
	"time"
)

// SessionStatus represents the lifecycle state of a session.
type SessionStatus string

const (
	StatusActive   SessionStatus = "active"
	StatusStopped  SessionStatus = "stopped"
	StatusStopping SessionStatus = "stopping"
)

// Session represents a Claude Code session tracked by Clade.
// Stored as ~/.clade/sessions/{session_id}.json
type Session struct {
	SessionID    string        `json:"session_id"`
	Project      string        `json:"project"`
	CWD          string        `json:"cwd"`
	Branch       string        `json:"branch"`
	Started      time.Time     `json:"started"`
	LastActive   time.Time     `json:"last_active"`
	Status       SessionStatus `json:"status"`
	Summary      string        `json:"summary,omitempty"`
	IsWorktree   bool          `json:"is_worktree"`
	WorktreeName string        `json:"worktree_name,omitempty"`
	TokensUsed   int64         `json:"tokens_used,omitempty"`

	// Inbox tracking: byte offset of last-read position in today's inbox file.
	// Used by inject-context to only show new entries.
	InboxReadOffset int64 `json:"inbox_read_offset,omitempty"`
}

const (
	// StaleThreshold is the duration after which an idle session is considered stale.
	StaleThreshold = 24 * time.Hour
	// ArchiveThreshold is the duration after which a stale session is auto-archived.
	ArchiveThreshold = 7 * 24 * time.Hour
	// StoppingTimeout is how long a "stopping" session waits before being considered stuck.
	StoppingTimeout = 5 * time.Minute
)

// IsStale returns true if the session has not been active for more than StaleThreshold.
func (s *Session) IsStale() bool {
	return time.Since(s.LastActive) > StaleThreshold
}

// IsArchivable returns true if the session has not been active for more than ArchiveThreshold.
func (s *Session) IsArchivable() bool {
	return time.Since(s.LastActive) > ArchiveThreshold
}

// StatusLabel returns a human-readable status string for dashboard display.
// Examples: "active", "idle 2h", "stale 1d", "stopping", "stopped (incomplete)"
func (s *Session) StatusLabel() string {
	switch s.Status {
	case StatusActive:
		return "active"
	case StatusStopping:
		if time.Since(s.LastActive) > StoppingTimeout {
			return "stopped (incomplete)"
		}
		return "stopping"
	case StatusStopped:
		age := time.Since(s.LastActive)
		if age > StaleThreshold {
			return fmt.Sprintf("stale %s", formatCompactDuration(age))
		}
		return fmt.Sprintf("idle %s", formatCompactDuration(age))
	default:
		return string(s.Status)
	}
}

// formatCompactDuration returns a compact age like "2h", "3d", "15m".
func formatCompactDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// InboxEntryType classifies an inbox message.
type InboxEntryType string

const (
	EntryDecision InboxEntryType = "decision"
	EntryFYI      InboxEntryType = "fyi"
	EntryBlocker  InboxEntryType = "blocker"
	EntryHandoff  InboxEntryType = "handoff"
)

// IsValid returns true if the entry type is one of the known types.
func (t InboxEntryType) IsValid() bool {
	switch t {
	case EntryDecision, EntryFYI, EntryBlocker, EntryHandoff:
		return true
	}
	return false
}

// InboxEntry represents a single entry in the daily inbox file.
type InboxEntry struct {
	Time      time.Time      `json:"-"` // parsed from markdown header
	Project   string         `json:"-"` // parsed from markdown header
	EntryType InboxEntryType `json:"-"` // parsed from markdown header
	Message   string         `json:"-"` // the content line(s)
	SessionID string         `json:"-"` // optional, for dedup
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/session/...`

- [ ] **Step 5: Commit**

---

### Task 2: Session Registry CRUD

**Files:**
- Create: `internal/session/registry.go`
- Test: `internal/session/registry_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/session/registry_test.go`:

```go
package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_CreateAndRead(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	sess := &Session{
		SessionID:  "test-session-123",
		Project:    "my-project",
		CWD:        "/home/user/my-project",
		Branch:     "main",
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     StatusActive,
	}

	err := reg.Save(sess)
	require.NoError(t, err)

	loaded, err := reg.Get("test-session-123")
	require.NoError(t, err)
	assert.Equal(t, sess.SessionID, loaded.SessionID)
	assert.Equal(t, sess.Project, loaded.Project)
	assert.Equal(t, sess.CWD, loaded.CWD)
	assert.Equal(t, StatusActive, loaded.Status)
}

func TestRegistry_Get_NotFound(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	_, err := reg.Get("nonexistent")
	assert.Error(t, err)
}

func TestRegistry_List(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	// Create 3 sessions
	for i, id := range []string{"sess-1", "sess-2", "sess-3"} {
		sess := &Session{
			SessionID:  id,
			Project:    "proj",
			CWD:        "/tmp",
			Started:    time.Now().Add(-time.Duration(i) * time.Hour),
			LastActive: time.Now().Add(-time.Duration(i) * time.Hour),
			Status:     StatusActive,
		}
		require.NoError(t, reg.Save(sess))
	}

	sessions, err := reg.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestRegistry_Update(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	sess := &Session{
		SessionID:  "update-test",
		Project:    "proj",
		CWD:        "/tmp",
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     StatusActive,
	}
	require.NoError(t, reg.Save(sess))

	// Update
	sess.Status = StatusStopped
	sess.Summary = "Did some work"
	require.NoError(t, reg.Save(sess))

	loaded, err := reg.Get("update-test")
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, loaded.Status)
	assert.Equal(t, "Did some work", loaded.Summary)
}

func TestRegistry_Archive(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	sess := &Session{
		SessionID:  "archive-me",
		Project:    "proj",
		CWD:        "/tmp",
		Started:    time.Now().Add(-10 * 24 * time.Hour),
		LastActive: time.Now().Add(-10 * 24 * time.Hour),
		Status:     StatusStopped,
	}
	require.NoError(t, reg.Save(sess))

	err := reg.Archive("archive-me")
	require.NoError(t, err)

	// Should no longer be in active list
	sessions, err := reg.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 0)

	// Should exist in archive directory
	archivePath := filepath.Join(dir, "sessions", "archive", "archive-me.json")
	_, err = os.Stat(archivePath)
	assert.NoError(t, err)
}

func TestRegistry_ArchiveStale(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	// Create one fresh and one archivable session
	fresh := &Session{
		SessionID:  "fresh",
		Project:    "proj",
		CWD:        "/tmp",
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     StatusActive,
	}
	old := &Session{
		SessionID:  "old",
		Project:    "proj",
		CWD:        "/tmp",
		Started:    time.Now().Add(-10 * 24 * time.Hour),
		LastActive: time.Now().Add(-10 * 24 * time.Hour),
		Status:     StatusStopped,
	}
	require.NoError(t, reg.Save(fresh))
	require.NoError(t, reg.Save(old))

	archived, err := reg.ArchiveStale()
	require.NoError(t, err)
	assert.Equal(t, 1, archived)

	sessions, err := reg.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "fresh", sessions[0].SessionID)
}

func TestRegistry_Delete(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	sess := &Session{
		SessionID:  "delete-me",
		Project:    "proj",
		CWD:        "/tmp",
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     StatusStopped,
	}
	require.NoError(t, reg.Save(sess))

	err := reg.Delete("delete-me")
	require.NoError(t, err)

	_, err = reg.Get("delete-me")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/session/...`

Expected: FAIL — `NewRegistry` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/session/registry.go`:

```go
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Registry manages session JSON files in a directory.
type Registry struct {
	baseDir string // e.g. ~/.clade/
}

// NewRegistry creates a Registry rooted at baseDir.
// Sessions are stored in baseDir/sessions/{id}.json
func NewRegistry(baseDir string) *Registry {
	return &Registry{baseDir: baseDir}
}

// sessionsDir returns the path to the sessions directory.
func (r *Registry) sessionsDir() string {
	return filepath.Join(r.baseDir, "sessions")
}

// archiveDir returns the path to the sessions archive directory.
func (r *Registry) archiveDir() string {
	return filepath.Join(r.baseDir, "sessions", "archive")
}

// sessionPath returns the file path for a session ID.
func (r *Registry) sessionPath(sessionID string) string {
	// Sanitize session ID to prevent path traversal
	safe := sanitizeID(sessionID)
	return filepath.Join(r.sessionsDir(), safe+".json")
}

// Save writes a session to disk. Creates or overwrites.
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

// Get reads a session by ID.
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

// List returns all active (non-archived) sessions, sorted by LastActive descending.
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
		if entry.IsDir() {
			continue // skip archive/ subdirectory
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(r.sessionsDir(), entry.Name()))
		if err != nil {
			continue // skip unreadable files
		}

		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue // skip corrupt files
		}

		sessions = append(sessions, &sess)
	}

	// Sort by LastActive descending (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActive.After(sessions[j].LastActive)
	})

	return sessions, nil
}

// Archive moves a session from active to archive directory.
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

// ArchiveStale archives all sessions that exceed the ArchiveThreshold.
// Returns the count of archived sessions.
func (r *Registry) ArchiveStale() (int, error) {
	sessions, err := r.List()
	if err != nil {
		return 0, err
	}

	archived := 0
	for _, sess := range sessions {
		if sess.IsArchivable() {
			if err := r.Archive(sess.SessionID); err != nil {
				continue // best-effort
			}
			archived++
		}
	}

	return archived, nil
}

// Delete removes a session file permanently.
func (r *Registry) Delete(sessionID string) error {
	path := r.sessionPath(sessionID)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete session %q: %w", sessionID, err)
	}
	return nil
}

// DropbagPath returns the path to a session's dropbag file.
// In v0.8, dropbags are stored as ~/.clade/sessions/{session_id}.md
func (r *Registry) DropbagPath(sessionID string) string {
	safe := sanitizeID(sessionID)
	return filepath.Join(r.sessionsDir(), safe+".md")
}

// sanitizeID removes path separators and null bytes from a session ID.
func sanitizeID(id string) string {
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "\\", "_")
	id = strings.ReplaceAll(id, "\x00", "")
	return id
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/session/...`

- [ ] **Step 5: Commit**

---

### Task 3: Inbox Read/Write

**Files:**
- Create: `internal/session/inbox.go`
- Test: `internal/session/inbox_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/session/inbox_test.go`:

```go
package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInbox_Append(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)

	entry := &InboxEntry{
		Time:      time.Date(2026, 4, 4, 10, 30, 0, 0, time.Local),
		Project:   "leap-complete",
		EntryType: EntryDecision,
		Message:   "API Gateway pointing at old ALB. JWT PRs separate.",
	}

	err := inbox.Append(entry)
	require.NoError(t, err)

	// Verify file was created with correct name
	today := time.Now().Format("2006-01-02")
	filePath := filepath.Join(dir, "inbox", today+".md")
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "leap-complete")
	assert.Contains(t, content, "decision")
	assert.Contains(t, content, "API Gateway pointing at old ALB")
}

func TestInbox_AppendMultiple(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)

	entries := []*InboxEntry{
		{
			Time:      time.Now(),
			Project:   "proj-a",
			EntryType: EntryDecision,
			Message:   "First entry",
		},
		{
			Time:      time.Now(),
			Project:   "proj-b",
			EntryType: EntryFYI,
			Message:   "Second entry",
		},
	}

	for _, e := range entries {
		require.NoError(t, inbox.Append(e))
	}

	// Verify both entries exist in same file
	today := time.Now().Format("2006-01-02")
	filePath := filepath.Join(dir, "inbox", today+".md")
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "First entry")
	assert.Contains(t, content, "Second entry")
}

func TestInbox_ReadToday(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)

	// Write an entry
	entry := &InboxEntry{
		Time:      time.Now(),
		Project:   "test-proj",
		EntryType: EntryFYI,
		Message:   "Lambda fix done",
	}
	require.NoError(t, inbox.Append(entry))

	// Read back
	entries, _, err := inbox.ReadRecent(0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 1)
	assert.Equal(t, "Lambda fix done", entries[0].Message)
}

func TestInbox_ReadSinceOffset(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)

	// Write first entry
	entry1 := &InboxEntry{
		Time:      time.Now(),
		Project:   "proj",
		EntryType: EntryFYI,
		Message:   "First",
	}
	require.NoError(t, inbox.Append(entry1))

	// Get current file size as offset
	today := time.Now().Format("2006-01-02")
	filePath := filepath.Join(dir, "inbox", today+".md")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	offset := info.Size()

	// Write second entry
	entry2 := &InboxEntry{
		Time:      time.Now(),
		Project:   "proj",
		EntryType: EntryDecision,
		Message:   "Second",
	}
	require.NoError(t, inbox.Append(entry2))

	// Read only entries after offset
	entries, _, err := inbox.ReadRecent(offset)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "Second", entries[0].Message)
}

func TestInbox_Cleanup(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)
	inboxDir := filepath.Join(dir, "inbox")
	require.NoError(t, os.MkdirAll(inboxDir, 0755))

	// Create a file that's 40 days old
	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	oldPath := filepath.Join(inboxDir, oldDate+".md")
	require.NoError(t, os.WriteFile(oldPath, []byte("old data"), 0644))

	// Create a file that's 5 days old
	recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	recentPath := filepath.Join(inboxDir, recentDate+".md")
	require.NoError(t, os.WriteFile(recentPath, []byte("recent data"), 0644))

	removed, err := inbox.Cleanup(30)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	// Old file should be gone
	_, err = os.Stat(oldPath)
	assert.True(t, os.IsNotExist(err))

	// Recent file should remain
	_, err = os.Stat(recentPath)
	assert.NoError(t, err)
}

func TestInbox_FormatEntry(t *testing.T) {
	entry := &InboxEntry{
		Time:      time.Date(2026, 4, 4, 10, 30, 0, 0, time.Local),
		Project:   "leap-complete",
		EntryType: EntryDecision,
		Message:   "Going with pg-boss for job queuing.",
	}

	formatted := FormatInboxEntry(entry)
	assert.True(t, strings.HasPrefix(formatted, "\n### "))
	assert.Contains(t, formatted, "leap-complete")
	assert.Contains(t, formatted, "decision")
	assert.Contains(t, formatted, "Going with pg-boss")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/session/...`

Expected: FAIL — `NewInbox` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/session/inbox.go`:

```go
package session

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// Inbox manages the daily append-only inbox files in ~/.clade/inbox/.
type Inbox struct {
	baseDir string // e.g. ~/.clade/
}

// NewInbox creates an Inbox rooted at baseDir.
func NewInbox(baseDir string) *Inbox {
	return &Inbox{baseDir: baseDir}
}

// inboxDir returns the path to the inbox directory.
func (ib *Inbox) inboxDir() string {
	return filepath.Join(ib.baseDir, "inbox")
}

// todayFile returns the path to today's inbox file.
func (ib *Inbox) todayFile() string {
	return filepath.Join(ib.inboxDir(), time.Now().Format("2006-01-02")+".md")
}

// yesterdayFile returns the path to yesterday's inbox file.
func (ib *Inbox) yesterdayFile() string {
	return filepath.Join(ib.inboxDir(), time.Now().AddDate(0, 0, -1).Format("2006-01-02")+".md")
}

// FormatInboxEntry formats an InboxEntry as markdown for appending to the inbox file.
func FormatInboxEntry(entry *InboxEntry) string {
	timeStr := entry.Time.Format("3:04 PM")
	return fmt.Sprintf("\n### %s | %s | %s\n%s\n", timeStr, entry.Project, entry.EntryType, entry.Message)
}

// Append writes an inbox entry to today's file with flock for concurrency safety.
func (ib *Inbox) Append(entry *InboxEntry) error {
	if err := os.MkdirAll(ib.inboxDir(), 0755); err != nil {
		return fmt.Errorf("failed to create inbox dir: %w", err)
	}

	filePath := ib.todayFile()
	formatted := FormatInboxEntry(entry)

	// Open file for appending (create if not exists)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open inbox file: %w", err)
	}
	defer f.Close()

	// flock for concurrency safety (two sessions appending simultaneously)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	// If file is empty, write the frontmatter header
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		today := time.Now().Format("2006-01-02")
		header := fmt.Sprintf("---\ndate: %s\n---\n", today)
		if _, err := f.WriteString(header); err != nil {
			return err
		}
	}

	if _, err := f.WriteString(formatted); err != nil {
		return fmt.Errorf("failed to write inbox entry: %w", err)
	}

	return nil
}

// entryHeaderRe matches inbox entry headers like "### 10:30 AM | proj | decision"
var entryHeaderRe = regexp.MustCompile(`^### (.+?) \| (.+?) \| (.+)$`)

// ReadRecent reads today's and yesterday's inbox entries.
// If offset > 0, only reads entries from today's file after that byte offset.
// Returns parsed entries and the new byte offset (current end of today's file).
func (ib *Inbox) ReadRecent(offset int64) ([]*InboxEntry, int64, error) {
	var allEntries []*InboxEntry

	// Read yesterday's file (always from start — it's a different day)
	if entries, err := ib.readFile(ib.yesterdayFile(), 0); err == nil {
		allEntries = append(allEntries, entries...)
	}

	// Read today's file from offset
	entries, err := ib.readFile(ib.todayFile(), offset)
	if err != nil && !os.IsNotExist(err) {
		return nil, 0, err
	}
	allEntries = append(allEntries, entries...)

	// Get new offset (current size of today's file)
	var newOffset int64
	if info, err := os.Stat(ib.todayFile()); err == nil {
		newOffset = info.Size()
	}

	return allEntries, newOffset, nil
}

// readFile reads inbox entries from a file, starting at the given byte offset.
func (ib *Inbox) readFile(path string, offset int64) ([]*InboxEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, err
		}
	}

	var entries []*InboxEntry
	var current *InboxEntry

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if matches := entryHeaderRe.FindStringSubmatch(line); matches != nil {
			// Save previous entry
			if current != nil {
				current.Message = strings.TrimSpace(current.Message)
				entries = append(entries, current)
			}

			current = &InboxEntry{
				Project:   strings.TrimSpace(matches[2]),
				EntryType: InboxEntryType(strings.TrimSpace(matches[3])),
				Message:   "",
			}

			// Parse time (best effort)
			if t, err := time.Parse("3:04 PM", strings.TrimSpace(matches[1])); err == nil {
				now := time.Now()
				current.Time = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
			}
			continue
		}

		// Skip frontmatter
		if line == "---" || strings.HasPrefix(line, "date:") {
			continue
		}

		// Accumulate message lines
		if current != nil {
			if current.Message != "" {
				current.Message += "\n"
			}
			current.Message += line
		}
	}

	// Don't forget last entry
	if current != nil {
		current.Message = strings.TrimSpace(current.Message)
		entries = append(entries, current)
	}

	return entries, scanner.Err()
}

// Cleanup removes inbox files older than maxDays.
// Returns the number of files removed.
func (ib *Inbox) Cleanup(maxDays int) (int, error) {
	entries, err := os.ReadDir(ib.inboxDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := time.Now().AddDate(0, 0, -maxDays)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		// Parse date from filename (YYYY-MM-DD.md)
		datePart := strings.TrimSuffix(entry.Name(), ".md")
		fileDate, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			continue // skip non-date files
		}

		if fileDate.Before(cutoff) {
			path := filepath.Join(ib.inboxDir(), entry.Name())
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
	}

	return removed, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/session/...`

- [ ] **Step 5: Commit**

---

### Task 4: session-start Command

**Files:**
- Create: `internal/cmd/session_start.go`
- Test: `internal/cmd/session_start_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/session_start_test.go`:

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniil-lyalko/clade/internal/session"
)

func TestDetectProjectName(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"simple dir", "/home/user/my-project", "my-project"},
		{"nested dir", "/home/user/code/my-project/src", "src"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectProjectName(tt.cwd)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSessionStart_CreatesSessionFile(t *testing.T) {
	tmpDir := t.TempDir()
	sessDir := filepath.Join(tmpDir, "sessions")

	reg := session.NewRegistry(tmpDir)

	input := &stopHookInput{
		SessionID: "test-sess-001",
		CWD:       "/home/user/my-project",
	}

	err := doSessionStart(reg, input)
	require.NoError(t, err)

	// Verify session file exists
	sessPath := filepath.Join(sessDir, "test-sess-001.json")
	data, err := os.ReadFile(sessPath)
	require.NoError(t, err)

	var sess session.Session
	require.NoError(t, json.Unmarshal(data, &sess))
	assert.Equal(t, "test-sess-001", sess.SessionID)
	assert.Equal(t, session.StatusActive, sess.Status)
	assert.Equal(t, "my-project", sess.Project)
	assert.Equal(t, "/home/user/my-project", sess.CWD)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestSessionStart -run TestDetectProjectName`

Expected: FAIL — `doSessionStart` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/cmd/session_start.go`:

```go
package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/spf13/cobra"
)

var sessionStartCmd = &cobra.Command{
	Use:    "session-start",
	Short:  "Register session in the Clade session registry (called by SessionStart hook)",
	Hidden: true,
	RunE:   runSessionStart,
}

func init() {
	rootCmd.AddCommand(sessionStartCmd)
}

func runSessionStart(cmd *cobra.Command, args []string) error {
	input, err := readStopHookInput()
	if err != nil {
		// No stdin (manual invocation) — exit silently
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)

	return doSessionStart(reg, input)
}

// doSessionStart creates or updates a session entry in the registry.
// Extracted for testability.
func doSessionStart(reg *session.Registry, input *stopHookInput) error {
	cwd := input.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	project := detectProjectName(cwd)
	branch := detectBranch(cwd)

	// Check if session already exists (resume case)
	existing, err := reg.Get(input.SessionID)
	if err == nil {
		// Resuming — update status and last_active
		existing.Status = session.StatusActive
		existing.LastActive = time.Now()
		if branch != "" {
			existing.Branch = branch
		}
		return reg.Save(existing)
	}

	// New session
	sess := &session.Session{
		SessionID:  input.SessionID,
		Project:    project,
		CWD:        cwd,
		Branch:     branch,
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     session.StatusActive,
	}

	// Detect if inside a clade worktree
	sess.IsWorktree, sess.WorktreeName = detectWorktree(cwd)

	return reg.Save(sess)
}

// detectProjectName extracts the project name from CWD.
// Uses the git remote origin name if available, otherwise the directory name.
func detectProjectName(cwd string) string {
	// Try git remote
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = cwd
	if out, err := cmd.Output(); err == nil {
		url := strings.TrimSpace(string(out))
		// Extract repo name from URL (handles both HTTPS and SSH)
		name := filepath.Base(url)
		name = strings.TrimSuffix(name, ".git")
		if name != "" && name != "." {
			return name
		}
	}

	return filepath.Base(cwd)
}

// detectBranch returns the current git branch for the directory.
func detectBranch(cwd string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = cwd
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// detectWorktree checks if the CWD is inside a clade-managed worktree.
// Returns (isWorktree, worktreeName).
func detectWorktree(cwd string) (bool, string) {
	// Check if inside a git worktree (not the main working tree)
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = cwd
	commonDir, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	cmd2 := exec.Command("git", "rev-parse", "--git-dir")
	cmd2.Dir = cwd
	gitDir, err := cmd2.Output()
	if err != nil {
		return false, ""
	}

	// If git-dir != git-common-dir, we're in a worktree
	if strings.TrimSpace(string(commonDir)) != strings.TrimSpace(string(gitDir)) {
		return true, filepath.Base(cwd)
	}

	return false, ""
}

// cladeBaseDir returns the base directory for Clade data (~/.clade/).
func cladeBaseDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".clade")
	}
	return filepath.Join(homeDir, ".clade")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run "TestSessionStart|TestDetectProjectName"`

- [ ] **Step 5: Commit**

---

### Task 5: session-stop Command with Triage

**Files:**
- Create: `internal/cmd/session_stop.go`
- Test: `internal/cmd/session_stop_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/session_stop_test.go`:

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniil-lyalko/clade/internal/session"
)

func TestTriageTier_Trivial(t *testing.T) {
	// No tool uses, < 3 user messages
	tier := triageSession(2, false, false, false)
	assert.Equal(t, tierTrivial, tier)
}

func TestTriageTier_Light(t *testing.T) {
	// Some activity but inbox entries were already written
	tier := triageSession(10, true, true, true)
	assert.Equal(t, tierLight, tier)
}

func TestTriageTier_NeedsAsync(t *testing.T) {
	// Edits happened but no inbox entries
	tier := triageSession(10, true, true, false)
	assert.Equal(t, tierAsync, tier)
}

func TestTriageTier_CommandsNoInbox(t *testing.T) {
	// Many commands but no inbox entries
	tier := triageSession(10, false, true, false)
	assert.Equal(t, tierAsync, tier)
}

func TestSessionStop_Trivial(t *testing.T) {
	tmpDir := t.TempDir()
	reg := session.NewRegistry(tmpDir)

	// Create an active session
	sess := &session.Session{
		SessionID: "trivial-sess",
		Project:   "test",
		CWD:       "/tmp",
		Status:    session.StatusActive,
	}
	require.NoError(t, reg.Save(sess))

	// Write a minimal transcript (just 2 user messages, no tool use)
	transcriptDir := t.TempDir()
	transcriptPath := filepath.Join(transcriptDir, "transcript.jsonl")
	writeMinimalTranscript(t, transcriptPath, 2, false, false)

	input := &stopHookInput{
		SessionID:      "trivial-sess",
		TranscriptPath: transcriptPath,
		CWD:            "/tmp",
	}

	err := doSessionStop(reg, session.NewInbox(tmpDir), input)
	require.NoError(t, err)

	loaded, err := reg.Get("trivial-sess")
	require.NoError(t, err)
	assert.Equal(t, session.StatusStopped, loaded.Status)
}

func writeMinimalTranscript(t *testing.T, path string, userMsgCount int, hasEdits, hasCommands bool) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	for i := 0; i < userMsgCount; i++ {
		entry := map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role":    "user",
				"content": "test message",
			},
		}
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.WriteString("\n")
	}

	if hasEdits {
		entry := map[string]interface{}{
			"type": "assistant",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "tool_use", "name": "Edit", "input": map[string]string{"file_path": "/tmp/foo.go"}},
				},
			},
		}
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.WriteString("\n")
	}

	if hasCommands {
		for i := 0; i < 4; i++ {
			entry := map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"role": "assistant",
					"content": []map[string]interface{}{
						{"type": "tool_use", "name": "Bash", "input": map[string]string{"command": "echo hello"}},
					},
				},
			}
			data, _ := json.Marshal(entry)
			f.Write(data)
			f.WriteString("\n")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run "TestTriage|TestSessionStop"`

Expected: FAIL — `triageSession` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/cmd/session_stop.go`:

```go
package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/spf13/cobra"
)

type triageTier int

const (
	tierTrivial triageTier = iota
	tierLight
	tierAsync
)

var sessionStopCmd = &cobra.Command{
	Use:    "session-stop",
	Short:  "Update session registry on stop (called by Stop hook)",
	Hidden: true,
	RunE:   runSessionStop,
}

func init() {
	rootCmd.AddCommand(sessionStopCmd)
}

func runSessionStop(cmd *cobra.Command, args []string) error {
	input, err := readStopHookInput()
	if err != nil {
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	// Loop guard
	if input.StopHookActive {
		return nil
	}

	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)
	inbox := session.NewInbox(baseDir)

	return doSessionStop(reg, inbox, input)
}

// doSessionStop performs the three-tier triage and updates the session.
func doSessionStop(reg *session.Registry, inbox *session.Inbox, input *stopHookInput) error {
	sess, err := reg.Get(input.SessionID)
	if err != nil {
		// Session not registered (possibly started before v0.8). Create a minimal record.
		sess = &session.Session{
			SessionID: input.SessionID,
			CWD:       input.CWD,
			Project:   detectProjectName(input.CWD),
			Started:   time.Now(),
		}
	}

	// Read last 5KB of transcript for triage
	userMsgs, hasEdits, hasCommands := quickTranscriptScan(input.TranscriptPath)

	// Check if inbox entries were already written during this session
	hasInbox := sessionHasInboxEntries(inbox, sess)

	tier := triageSession(userMsgs, hasEdits, hasCommands, hasInbox)

	switch tier {
	case tierTrivial:
		// Just mark stopped. <10ms.
		sess.Status = session.StatusStopped
		sess.LastActive = time.Now()
		return reg.Save(sess)

	case tierLight:
		// Update status + grab summary from last assistant message. <50ms.
		sess.Status = session.StatusStopped
		sess.LastActive = time.Now()
		sess.Summary = extractQuickSummary(input.TranscriptPath)
		return reg.Save(sess)

	case tierAsync:
		// Mark as stopping, fork background process, return immediately. <50ms.
		sess.Status = session.StatusStopping
		sess.LastActive = time.Now()
		if err := reg.Save(sess); err != nil {
			return err
		}
		return forkAsync(input)
	}

	return nil
}

// triageSession determines the processing tier for the stop hook.
func triageSession(userMsgs int, hasEdits, hasCommands, hasInbox bool) triageTier {
	trivial := userMsgs < 3 && !hasEdits && !hasCommands
	if trivial {
		return tierTrivial
	}

	needsAsync := (hasEdits || hasCommands) && !hasInbox
	if needsAsync {
		return tierAsync
	}

	return tierLight
}

// quickTranscriptScan reads the last 5KB of the transcript JSONL and extracts
// triage heuristics: user message count, presence of edits, presence of commands (>3 Bash).
func quickTranscriptScan(path string) (userMsgs int, hasEdits bool, hasCommands bool) {
	if path == "" {
		return 0, false, false
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, false, false
	}
	defer f.Close()

	// Seek to last 5KB
	const tailSize = 5 * 1024
	info, err := f.Stat()
	if err != nil {
		return 0, false, false
	}
	if info.Size() > tailSize {
		f.Seek(info.Size()-tailSize, io.SeekStart)
		// Skip partial first line
		reader := bufio.NewReader(f)
		reader.ReadString('\n')
		scanTail(reader, &userMsgs, &hasEdits, &hasCommands)
	} else {
		reader := bufio.NewReader(f)
		scanTail(reader, &userMsgs, &hasEdits, &hasCommands)
	}

	return
}

func scanTail(reader *bufio.Reader, userMsgs *int, hasEdits *bool, hasCommands *bool) {
	bashCount := 0
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Lightweight JSON check without full unmarshal
		lineStr := string(line)
		if strings.Contains(lineStr, `"type":"user"`) || strings.Contains(lineStr, `"type": "user"`) {
			*userMsgs++
		}
		if strings.Contains(lineStr, `"name":"Edit"`) || strings.Contains(lineStr, `"name": "Edit"`) ||
			strings.Contains(lineStr, `"name":"Write"`) || strings.Contains(lineStr, `"name": "Write"`) {
			*hasEdits = true
		}
		if strings.Contains(lineStr, `"name":"Bash"`) || strings.Contains(lineStr, `"name": "Bash"`) {
			bashCount++
		}
	}

	*hasCommands = bashCount > 3
}

// sessionHasInboxEntries checks if today's inbox file contains entries from this session's project.
func sessionHasInboxEntries(inbox *session.Inbox, sess *session.Session) bool {
	entries, _, err := inbox.ReadRecent(0)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Project == sess.Project {
			return true
		}
	}
	return false
}

// extractQuickSummary reads the last few KB of transcript and returns the
// last assistant text message as a summary.
func extractQuickSummary(path string) string {
	if path == "" {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	const tailSize = 10 * 1024
	info, err := f.Stat()
	if err != nil {
		return ""
	}

	var reader *bufio.Reader
	if info.Size() > tailSize {
		f.Seek(info.Size()-tailSize, io.SeekStart)
		reader = bufio.NewReader(f)
		reader.ReadString('\n') // skip partial line
	} else {
		reader = bufio.NewReader(f)
	}

	var lastAssistantText string
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "assistant" {
			continue
		}

		// Try content as array of blocks
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(entry.Message.Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
					lastAssistantText = b.Text
				}
			}
			continue
		}

		// Try content as string
		var s string
		if err := json.Unmarshal(entry.Message.Content, &s); err == nil && s != "" {
			lastAssistantText = s
		}
	}

	// Truncate to 300 chars
	if len(lastAssistantText) > 300 {
		lastAssistantText = lastAssistantText[:300] + "..."
	}

	return lastAssistantText
}

// forkAsync launches `clade session-stop-async` as a detached background process.
func forkAsync(input *stopHookInput) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find clade executable: %w", err)
	}

	// Pass session info as JSON via env var (stdin won't be available to background process)
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "session-stop-async")
	cmd.Env = append(os.Environ(), "CLADE_STOP_INPUT="+string(inputJSON))
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Detach from parent process group
	cmd.SysProcAttr = detachedProcAttr()

	return cmd.Start()
}
```

Also create `internal/cmd/procattr_unix.go` for the detached process attribute (build-tagged):

```go
//go:build !windows

package cmd

import "syscall"

func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
```

And `internal/cmd/procattr_windows.go`:

```go
//go:build windows

package cmd

import "syscall"

func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // CREATE_NO_WINDOW
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run "TestTriage|TestSessionStop"`

- [ ] **Step 5: Commit**

---

### Task 6: session-stop-async Command

**Files:**
- Create: `internal/cmd/session_stop_async.go`
- Test: `internal/cmd/session_stop_async_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/session_stop_async_test.go`:

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniil-lyalko/clade/internal/session"
)

func TestSessionStopAsync_UpdatesSession(t *testing.T) {
	tmpDir := t.TempDir()
	reg := session.NewRegistry(tmpDir)
	inbox := session.NewInbox(tmpDir)

	// Create session in "stopping" state
	sess := &session.Session{
		SessionID:  "async-test",
		Project:    "proj",
		CWD:        "/tmp",
		Started:    time.Now().Add(-1 * time.Hour),
		LastActive: time.Now(),
		Status:     session.StatusStopping,
	}
	require.NoError(t, reg.Save(sess))

	// Create a transcript with some content
	transcriptDir := t.TempDir()
	transcriptPath := filepath.Join(transcriptDir, "transcript.jsonl")
	writeMinimalTranscript(t, transcriptPath, 5, true, true)

	input := &stopHookInput{
		SessionID:      "async-test",
		TranscriptPath: transcriptPath,
		CWD:            "/tmp",
	}

	err := doSessionStopAsync(reg, inbox, input)
	require.NoError(t, err)

	// Verify session is now stopped
	loaded, err := reg.Get("async-test")
	require.NoError(t, err)
	assert.Equal(t, session.StatusStopped, loaded.Status)
}

func TestSessionStopAsync_WritesInboxFYI(t *testing.T) {
	tmpDir := t.TempDir()
	reg := session.NewRegistry(tmpDir)
	inbox := session.NewInbox(tmpDir)

	sess := &session.Session{
		SessionID:  "inbox-test",
		Project:    "my-proj",
		CWD:        "/tmp",
		Started:    time.Now().Add(-1 * time.Hour),
		LastActive: time.Now(),
		Status:     session.StatusStopping,
	}
	require.NoError(t, reg.Save(sess))

	// Create transcript with assistant text
	transcriptDir := t.TempDir()
	transcriptPath := filepath.Join(transcriptDir, "transcript.jsonl")
	f, err := os.Create(transcriptPath)
	require.NoError(t, err)
	entry := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": "I fixed the Lambda timeout issue and pushed the changes."},
			},
		},
	}
	data, _ := json.Marshal(entry)
	f.Write(data)
	f.WriteString("\n")
	f.Close()

	input := &stopHookInput{
		SessionID:      "inbox-test",
		TranscriptPath: transcriptPath,
		CWD:            "/tmp",
	}

	err = doSessionStopAsync(reg, inbox, input)
	require.NoError(t, err)

	// Check inbox has an FYI entry
	entries, _, err := inbox.ReadRecent(0)
	require.NoError(t, err)
	found := false
	for _, e := range entries {
		if e.Project == "my-proj" && e.EntryType == session.EntryFYI {
			found = true
			break
		}
	}
	assert.True(t, found, "expected FYI inbox entry for my-proj")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestSessionStopAsync`

Expected: FAIL — `doSessionStopAsync` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/cmd/session_stop_async.go`:

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/transcript"
	"github.com/spf13/cobra"
)

var sessionStopAsyncCmd = &cobra.Command{
	Use:    "session-stop-async",
	Short:  "Background async stop processing (called by session-stop)",
	Hidden: true,
	RunE:   runSessionStopAsync,
}

func init() {
	rootCmd.AddCommand(sessionStopAsyncCmd)
}

func runSessionStopAsync(cmd *cobra.Command, args []string) error {
	// Read input from CLADE_STOP_INPUT env var (stdin not available in background)
	inputJSON := os.Getenv("CLADE_STOP_INPUT")
	if inputJSON == "" {
		return nil
	}

	var input stopHookInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)
	inbox := session.NewInbox(baseDir)

	return doSessionStopAsync(reg, inbox, &input)
}

// doSessionStopAsync performs the full transcript parse, writes dropbag + inbox FYI,
// and marks the session as stopped. This runs as a background process.
func doSessionStopAsync(reg *session.Registry, inbox *session.Inbox, input *stopHookInput) error {
	sess, err := reg.Get(input.SessionID)
	if err != nil {
		// Session was deleted/archived while we were backgrounded
		return nil
	}

	var summary string

	// 1. Parse transcript
	if input.TranscriptPath != "" {
		extract, err := transcript.Parse(input.TranscriptPath)
		if err == nil && extract.TotalToolUses > 0 {
			// 2. Generate dropbag content
			content := transcript.FormatMarkdown(extract)

			// 3. Write session dropbag to ~/.clade/sessions/{session_id}.md
			dropbagPath := reg.DropbagPath(input.SessionID)
			os.MkdirAll(filepath.Dir(dropbagPath), 0755)
			os.WriteFile(dropbagPath, []byte(content), 0644)

			// Build summary from extract
			if extract.UserIntent != "" {
				summary = extract.UserIntent
				if len(summary) > 200 {
					summary = summary[:200] + "..."
				}
			}
		}
	}

	// 4. Append FYI entry to today's inbox (safety net for sessions that didn't broadcast)
	if summary == "" {
		summary = extractQuickSummary(input.TranscriptPath)
	}
	if summary != "" {
		entry := &session.InboxEntry{
			Time:      time.Now(),
			Project:   sess.Project,
			EntryType: session.EntryFYI,
			Message:   summary,
			SessionID: input.SessionID,
		}
		inbox.Append(entry) // best-effort, ignore errors
	}

	// 5. Update session to stopped with final summary
	sess.Status = session.StatusStopped
	sess.LastActive = time.Now()
	if summary != "" {
		sess.Summary = summary
	}

	return reg.Save(sess)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestSessionStopAsync`

- [ ] **Step 5: Commit**

---

### Task 7: session-compact Command

**Files:**
- Create: `internal/cmd/session_compact.go`
- Test: `internal/cmd/session_compact_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/session_compact_test.go`:

```go
package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniil-lyalko/clade/internal/session"
)

func TestSessionCompact_UpdatesSummary(t *testing.T) {
	tmpDir := t.TempDir()
	reg := session.NewRegistry(tmpDir)

	sess := &session.Session{
		SessionID:  "compact-test",
		Project:    "proj",
		CWD:        "/tmp",
		Started:    time.Now().Add(-2 * time.Hour),
		LastActive: time.Now().Add(-1 * time.Hour),
		Status:     session.StatusActive,
		TokensUsed: 50000,
	}
	require.NoError(t, reg.Save(sess))

	input := &stopHookInput{
		SessionID: "compact-test",
		CWD:       "/tmp",
	}

	err := doSessionCompact(reg, input)
	require.NoError(t, err)

	loaded, err := reg.Get("compact-test")
	require.NoError(t, err)
	assert.Equal(t, session.StatusActive, loaded.Status) // still active
	assert.True(t, loaded.LastActive.After(sess.LastActive))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestSessionCompact`

Expected: FAIL — `doSessionCompact` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/cmd/session_compact.go`:

```go
package cmd

import (
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/spf13/cobra"
)

var sessionCompactCmd = &cobra.Command{
	Use:    "session-compact",
	Short:  "Update session on compaction (called by PreCompact hook)",
	Hidden: true,
	RunE:   runSessionCompact,
}

func init() {
	rootCmd.AddCommand(sessionCompactCmd)
}

func runSessionCompact(cmd *cobra.Command, args []string) error {
	input, err := readStopHookInput()
	if err != nil {
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)

	return doSessionCompact(reg, input)
}

// doSessionCompact updates the session's summary and token count on compaction.
func doSessionCompact(reg *session.Registry, input *stopHookInput) error {
	sess, err := reg.Get(input.SessionID)
	if err != nil {
		// Session not registered, create minimal
		sess = &session.Session{
			SessionID: input.SessionID,
			CWD:       input.CWD,
			Project:   detectProjectName(input.CWD),
			Started:   time.Now(),
			Status:    session.StatusActive,
		}
	}

	// Update last active time
	sess.LastActive = time.Now()

	// Try to grab a quick summary from the transcript tail
	if input.TranscriptPath != "" {
		if summary := extractQuickSummary(input.TranscriptPath); summary != "" {
			sess.Summary = summary
		}
	}

	return reg.Save(sess)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestSessionCompact`

- [ ] **Step 5: Commit**

---

### Task 8: `clade sessions` Interactive Dashboard

**Files:**
- Create: `internal/cmd/sessions.go`
- Test: `internal/cmd/sessions_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/sessions_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniil-lyalko/clade/internal/session"
)

func TestFormatSessionsDashboard(t *testing.T) {
	sessions := []*session.Session{
		{
			SessionID:  "sess-1",
			Project:    "leap-complete",
			Status:     session.StatusStopped,
			LastActive: time.Now().Add(-2 * time.Hour),
			Summary:    "API gateway debug",
		},
		{
			SessionID:  "sess-2",
			Project:    "document-engine",
			Status:     session.StatusStopped,
			LastActive: time.Now().Add(-4 * time.Hour),
			Summary:    "Lambda fix DONE",
		},
	}

	var buf bytes.Buffer
	formatSessionsDashboard(&buf, sessions)
	output := buf.String()

	assert.Contains(t, output, "leap-complete")
	assert.Contains(t, output, "document-engine")
	assert.Contains(t, output, "API gateway debug")
	assert.Contains(t, output, "Lambda fix DONE")
	assert.Contains(t, output, "2 sessions")
}

func TestFormatSessionsJSON(t *testing.T) {
	sessions := []*session.Session{
		{
			SessionID:  "sess-1",
			Project:    "my-proj",
			CWD:        "/home/user/my-proj",
			Status:     session.StatusActive,
			LastActive: time.Now(),
			Summary:    "Working on stuff",
		},
	}

	var buf bytes.Buffer
	err := formatSessionsJSON(&buf, sessions)
	require.NoError(t, err)

	// Verify valid JSON
	var result []json.RawMessage
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Len(t, result, 1)
}

func TestCountSessionsByStatus(t *testing.T) {
	sessions := []*session.Session{
		{Status: session.StatusActive, LastActive: time.Now()},
		{Status: session.StatusStopped, LastActive: time.Now().Add(-2 * time.Hour)},
		{Status: session.StatusStopped, LastActive: time.Now().Add(-25 * time.Hour)},
	}

	active, idle, stale := countSessionsByStatus(sessions)
	assert.Equal(t, 1, active)
	assert.Equal(t, 1, idle)
	assert.Equal(t, 1, stale)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run "TestFormatSessions|TestCountSessions"`

Expected: FAIL — `formatSessionsDashboard` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/cmd/sessions.go`:

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	sessionsJSONFlag        bool
	sessionsActiveFlag      bool
	sessionsCleanFlag       bool
	sessionsNoInteractive   bool
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Show all Claude Code sessions (dashboard)",
	Long: `Interactive dashboard showing all active, idle, and stale Claude Code sessions.

Pick a session to resume, or clean up stale ones.

Flags:
  --json            JSON output for scripting
  --active          Only show active/idle sessions
  --clean           Archive all stale sessions
  --no-interactive  Print dashboard and exit`,
	RunE: runSessions,
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
	sessionsCmd.Flags().BoolVar(&sessionsJSONFlag, "json", false, "Output as JSON")
	sessionsCmd.Flags().BoolVar(&sessionsActiveFlag, "active", false, "Only show active/idle sessions")
	sessionsCmd.Flags().BoolVar(&sessionsCleanFlag, "clean", false, "Archive all stale sessions")
	sessionsCmd.Flags().BoolVar(&sessionsNoInteractive, "no-interactive", false, "Print dashboard and exit")
}

func runSessions(cmd *cobra.Command, args []string) error {
	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)

	// Handle --clean
	if sessionsCleanFlag {
		archived, err := reg.ArchiveStale()
		if err != nil {
			return fmt.Errorf("failed to archive stale sessions: %w", err)
		}
		if archived > 0 {
			ui.Success("Archived %d stale session(s)", archived)
		} else {
			ui.Info("No stale sessions to archive")
		}

		// Also clean up old inbox files
		inbox := session.NewInbox(baseDir)
		removed, _ := inbox.Cleanup(30)
		if removed > 0 {
			ui.Success("Removed %d old inbox file(s)", removed)
		}
		return nil
	}

	sessions, err := reg.List()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Filter active-only
	if sessionsActiveFlag {
		var filtered []*session.Session
		for _, s := range sessions {
			if !s.IsStale() {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	// JSON output
	if sessionsJSONFlag {
		return formatSessionsJSON(os.Stdout, sessions)
	}

	if len(sessions) == 0 {
		ui.Info("No active sessions")
		ui.Detail("Start a Claude Code session and it will appear here automatically")
		return nil
	}

	// Print dashboard
	formatSessionsDashboard(os.Stdout, sessions)

	// Interactive mode (default unless --no-interactive)
	if sessionsNoInteractive {
		return nil
	}

	return interactiveSessionPicker(reg, sessions)
}

// formatSessionsDashboard writes the formatted session table to w.
func formatSessionsDashboard(w io.Writer, sessions []*session.Session) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-4s %-12s %-18s %-6s %s\n",
		"", "STATUS", "PROJECT", "AGE", "DOING")

	for i, s := range sessions {
		num := fmt.Sprintf("%d", i+1)
		status := s.StatusLabel()
		age := formatCompactAge(s.LastActive)

		summary := s.Summary
		if len(summary) > 50 {
			summary = summary[:50] + "..."
		}

		// Color the status indicator
		var statusColor string
		switch {
		case s.Status == session.StatusActive:
			statusColor = ui.Green(fmt.Sprintf("● %s", status))
		case s.IsStale():
			statusColor = ui.Red(fmt.Sprintf("● %s", status))
		default:
			statusColor = ui.Yellow(fmt.Sprintf("● %s", status))
		}

		fmt.Fprintf(w, "  %-4s %-12s %-18s %-6s %s\n",
			num, statusColor, s.Project, ui.Dim(age), summary)
	}

	active, idle, stale := countSessionsByStatus(sessions)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %d sessions (%d active, %d idle, %d stale)\n",
		len(sessions), active, idle, stale)
	fmt.Fprintln(w)
}

// formatSessionsJSON writes sessions as JSON array to w.
func formatSessionsJSON(w io.Writer, sessions []*session.Session) error {
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	fmt.Fprintln(w)
	return nil
}

// countSessionsByStatus counts active, idle (<24h), and stale (>24h) sessions.
func countSessionsByStatus(sessions []*session.Session) (active, idle, stale int) {
	for _, s := range sessions {
		switch {
		case s.Status == session.StatusActive:
			active++
		case s.IsStale():
			stale++
		default:
			idle++
		}
	}
	return
}

// formatCompactAge returns a compact age string like "2h", "3d".
func formatCompactAge(t interface{ IsZero() bool }) string {
	// This is a workaround because time.Time doesn't have IsZero in an interface.
	// We just use the formatAgeShort from list.go pattern.
	return "" // placeholder
}

// Override with actual time.Time version:
func init() {
	// Intentionally left empty — formatCompactAge is defined below
}

// interactiveSessionPicker shows the resume prompt.
func interactiveSessionPicker(reg *session.Registry, sessions []*session.Session) error {
	var items []string
	for i, s := range sessions {
		items = append(items, fmt.Sprintf("%d: %s (%s)", i+1, s.Project, s.StatusLabel()))
	}
	items = append(items, "c: Clean stale sessions")
	items = append(items, "q: Quit")

	prompt := promptui.Select{
		Label: "Pick an action",
		Items: items,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return nil // user cancelled
	}

	// Clean stale
	if idx == len(items)-2 {
		archived, err := reg.ArchiveStale()
		if err != nil {
			return err
		}
		ui.Success("Archived %d stale session(s)", archived)
		return nil
	}

	// Quit
	if idx == len(items)-1 {
		return nil
	}

	// Resume session
	if idx < len(sessions) {
		return resumeSession(sessions[idx])
	}

	return nil
}

// resumeSession launches `claude --resume <session_id>` in the session's CWD.
func resumeSession(sess *session.Session) error {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found: %w", err)
	}

	ui.Info("Resuming session in %s", sess.CWD)

	cmd := exec.Command(claudePath, "--resume", sess.SessionID)
	cmd.Dir = sess.CWD
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
```

Note: The `formatCompactAge` function needs a proper implementation. Replace the placeholder with:

```go
// Remove the interface-based formatCompactAge and use time.Time directly.
// The dashboard calls it with session.LastActive which is time.Time.
// In formatSessionsDashboard, replace formatCompactAge(s.LastActive) with:
//   formatAgeShort(s.LastActive) — reuse from list.go
// Or duplicate the logic inline. Since formatAgeShort is in list.go (same package), it's directly accessible.
```

Actually, since both `sessions.go` and `list.go` are in `package cmd`, `formatAgeShort` from `list.go` is directly available. Remove the `formatCompactAge` function entirely and use `formatAgeShort(s.LastActive)` in `formatSessionsDashboard`. The final code in `formatSessionsDashboard` should use:

```go
age := formatAgeShort(s.LastActive)
```

Remove both `formatCompactAge` definitions and the second `init()` block.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run "TestFormatSessions|TestCountSessions"`

- [ ] **Step 5: Commit**

---

### Task 9: Update inject-context to Scan Inbox

**Files:**
- Modify: `internal/cmd/inject.go`
- Modify: `internal/context/format.go`
- Test: `internal/cmd/inject_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/inject_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniil-lyalko/clade/internal/session"
)

func TestFormatInboxContext(t *testing.T) {
	entries := []*session.InboxEntry{
		{
			Time:      time.Now(),
			Project:   "leap-complete",
			EntryType: session.EntryDecision,
			Message:   "API Gateway fix confirmed",
		},
		{
			Time:      time.Now(),
			Project:   "document-engine",
			EntryType: session.EntryFYI,
			Message:   "Lambda timeout fix done",
		},
	}

	output := formatInboxContext(entries)
	assert.Contains(t, output, "Cross-session updates")
	assert.Contains(t, output, "leap-complete")
	assert.Contains(t, output, "API Gateway fix confirmed")
	assert.Contains(t, output, "document-engine")
}

func TestFormatInboxContext_Empty(t *testing.T) {
	output := formatInboxContext(nil)
	assert.Equal(t, "", output)
}

func TestReadSessionDropbag(t *testing.T) {
	tmpDir := t.TempDir()
	sessDir := filepath.Join(tmpDir, "sessions")
	require.NoError(t, os.MkdirAll(sessDir, 0755))

	// Write a dropbag
	content := "# Session Context\n\nSome context here."
	dropbagPath := filepath.Join(sessDir, "test-session.md")
	require.NoError(t, os.WriteFile(dropbagPath, []byte(content), 0644))

	reg := session.NewRegistry(tmpDir)
	data, err := os.ReadFile(reg.DropbagPath("test-session"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Some context here")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run "TestFormatInbox|TestReadSessionDropbag"`

Expected: FAIL — `formatInboxContext` undefined

- [ ] **Step 3: Write minimal implementation**

Modify `internal/cmd/inject.go` — add the inbox scanning logic. Add these functions and modify `runInjectContext`:

```go
// Add to imports:
//   "github.com/daniil-lyalko/clade/internal/session"

// Add new function:
func formatInboxContext(entries []*session.InboxEntry) string {
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Cross-session updates\n\n")
	sb.WriteString("_Recent activity from other Claude Code sessions:_\n\n")

	for _, e := range entries {
		timeStr := ""
		if !e.Time.IsZero() {
			timeStr = e.Time.Format("3:04 PM")
		}
		sb.WriteString(fmt.Sprintf("- **[%s]** %s (%s): %s\n", e.EntryType, e.Project, timeStr, e.Message))
	}
	sb.WriteString("\n")

	return sb.String()
}
```

Modify `runInjectContext` to append inbox context after the existing context output. Insert before the output format detection block:

```go
	// Scan inbox for cross-session updates
	baseDir := cladeBaseDir()
	inbox := session.NewInbox(baseDir)

	// Read stdin to get session_id for offset tracking (best effort)
	var inboxOffset int64
	// For now, read all recent entries (offset tracking per-session is a future optimization)
	entries, _, err := inbox.ReadRecent(inboxOffset)
	if err == nil && len(entries) > 0 {
		inboxSection := formatInboxContext(entries)
		if inboxSection != "" {
			output = output + "\n" + inboxSection
		}
	}
```

Also read the session's centralized dropbag if a session ID is available. Add after the inbox scan:

```go
	// Read session dropbag from ~/.clade/sessions/{session_id}.md
	reg := session.NewRegistry(baseDir)
	if sessionID := os.Getenv("CLAUDE_SESSION_ID"); sessionID != "" {
		dropbagPath := reg.DropbagPath(sessionID)
		if data, err := os.ReadFile(dropbagPath); err == nil {
			content := strings.TrimSpace(string(data))
			if content != "" {
				output = "## Session Context (from previous session)\n\n" + content + "\n\n" + output
			}
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run "TestFormatInbox|TestReadSessionDropbag"`

- [ ] **Step 5: Commit**

---

### Task 10: Update setup Command for New Hooks

**Files:**
- Modify: `internal/cmd/setup.go`
- Test: `internal/cmd/setup_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/setup_test.go`:

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeClaudeSettingsHooks_V08SessionHooks(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	// Write empty settings
	require.NoError(t, os.WriteFile(settingsPath, []byte("{}"), 0644))

	_, err := mergeClaudeSettingsHooks(settingsPath, true)
	require.NoError(t, err)

	// Read back and verify all three hook types
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &root))

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(root["hooks"], &hooks))

	// SessionStart should have inject-context AND session-start
	assert.Contains(t, string(hooks["SessionStart"]), "clade inject-context")
	assert.Contains(t, string(hooks["SessionStart"]), "clade session-start")

	// Stop should have session-stop
	assert.Contains(t, string(hooks["Stop"]), "clade session-stop")

	// PreCompact should have session-compact
	assert.Contains(t, string(hooks["PreCompact"]), "clade session-compact")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestMergeClaudeSettingsHooks_V08`

Expected: FAIL — the current `mergeClaudeSettingsHooks` does not register `session-start`, `session-stop`, or `session-compact`

- [ ] **Step 3: Write minimal implementation**

Modify `internal/cmd/setup.go`:

1. In `planClaudeHookActions`, update the `hooks` slice to include the new v0.8 hook commands:

```go
	hooks := []hookDef{
		{"SessionStart hook (inject-context)", "clade inject-context", "pacer inject-context"},
		{"SessionStart hook (session-start)", "command -v clade >/dev/null 2>&1 && clade session-start || true", ""},
		{"Stop hook (session-stop)", "command -v clade >/dev/null 2>&1 && clade session-stop || true", ""},
		{"PreCompact hook (session-compact)", "command -v clade >/dev/null 2>&1 && clade session-compact || true", ""},
		{"PreCompact hook (context-warning)", "clade context-warning", ""},
	}
```

2. In `mergeClaudeSettingsHooks`, add the new hooks after the existing SessionStart block:

```go
	// SessionStart: session-start hook
	const sessionStartCmd = "command -v clade >/dev/null 2>&1 && clade session-start || true"
	mergeHookArray(hooksObj, "SessionStart", sessionStartCmd)

	// Stop hook: session-stop (replaces auto-dropbag)
	const sessionStopCmd = "command -v clade >/dev/null 2>&1 && clade session-stop || true"
	mergeHookArray(hooksObj, "Stop", sessionStopCmd)

	// PreCompact hook: session-compact
	const sessionCompactCmd = "command -v clade >/dev/null 2>&1 && clade session-compact || true"
	mergeHookArray(hooksObj, "PreCompact", sessionCompactCmd)
```

The existing `auto-dropbag` Stop hook should be kept for backward compatibility (session-stop calls it internally), or removed if session-stop fully replaces it. Based on the spec ("session-stop calls auto-dropbag internally"), remove the old auto-dropbag Stop hook registration and replace with session-stop. Keep the auto-dropbag PreCompact hook as session-compact replaces it too.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestMergeClaudeSettingsHooks_V08`

- [ ] **Step 5: Commit**

---

### Task 11: Update Config Paths to ~/.clade/

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/state.go`
- Test: `internal/config/config_test.go` (update existing)

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestDotCladeDir(t *testing.T) {
	dir := DotCladeDir()
	homeDir, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(homeDir, ".clade"), dir)
}

func TestDotCladeDir_SessionsPath(t *testing.T) {
	dir := DotCladeDir()
	sessionsDir := filepath.Join(dir, "sessions")
	assert.Contains(t, sessionsDir, ".clade/sessions")
}

func TestDotCladeDir_InboxPath(t *testing.T) {
	dir := DotCladeDir()
	inboxDir := filepath.Join(dir, "inbox")
	assert.Contains(t, inboxDir, ".clade/inbox")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/config/ -run TestDotClade`

Expected: FAIL — `DotCladeDir` undefined

- [ ] **Step 3: Write minimal implementation**

Add to `internal/config/config.go`:

```go
// DotCladeDir returns the path to ~/.clade/ — the unified Clade data directory.
// This is the new canonical location for all Clade state (v0.8+).
func DotCladeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".clade")
	}
	return filepath.Join(homeDir, ".clade")
}

// SessionsDir returns the path to ~/.clade/sessions/
func SessionsDir() string {
	return filepath.Join(DotCladeDir(), "sessions")
}

// InboxDir returns the path to ~/.clade/inbox/
func InboxDir() string {
	return filepath.Join(DotCladeDir(), "inbox")
}
```

Note: The existing `ConfigPath()` (returns `~/.config/clade/config.json`) and `StatePath()` (returns `~/.config/clade/state.json`) are **not changed yet** — they will be migrated by the migration command (Task 12). The new `DotCladeDir()` function is used by the session registry and inbox. After migration runs, `ConfigPath()` and `StatePath()` will also point to `~/.clade/`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/config/ -run TestDotClade`

- [ ] **Step 5: Commit**

---

### Task 12: Migration Command

**Files:**
- Create: `internal/cmd/migrate_dotclade.go`
- Test: `internal/cmd/migrate_dotclade_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/migrate_dotclade_test.go`:

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateToDotClade_MovesConfig(t *testing.T) {
	tmpHome := t.TempDir()

	// Create old config location
	oldConfigDir := filepath.Join(tmpHome, ".config", "clade")
	require.NoError(t, os.MkdirAll(oldConfigDir, 0755))

	cfg := map[string]interface{}{"agent": "claude", "base_dir": filepath.Join(tmpHome, "clade")}
	data, _ := json.Marshal(cfg)
	require.NoError(t, os.WriteFile(filepath.Join(oldConfigDir, "config.json"), data, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(oldConfigDir, "state.json"), []byte(`{"version":2}`), 0600))

	// Create old base_dir
	oldBaseDir := filepath.Join(tmpHome, "clade")
	require.NoError(t, os.MkdirAll(filepath.Join(oldBaseDir, "repos"), 0755))

	// Run migration
	newDir := filepath.Join(tmpHome, ".clade")
	err := doMigrateToDotClade(tmpHome, oldConfigDir, oldBaseDir, newDir)
	require.NoError(t, err)

	// Verify new location has files
	assert.FileExists(t, filepath.Join(newDir, "config.json"))
	assert.FileExists(t, filepath.Join(newDir, "state.json"))

	// Verify old location has symlink
	linkTarget, err := os.Readlink(filepath.Join(oldConfigDir, "config.json"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(newDir, "config.json"), linkTarget)
}

func TestMigrateToDotClade_CreatesDirectories(t *testing.T) {
	tmpHome := t.TempDir()
	newDir := filepath.Join(tmpHome, ".clade")

	err := doMigrateToDotClade(tmpHome, "", "", newDir)
	require.NoError(t, err)

	assert.DirExists(t, filepath.Join(newDir, "sessions"))
	assert.DirExists(t, filepath.Join(newDir, "inbox"))
}

func TestMigrateToDotClade_Idempotent(t *testing.T) {
	tmpHome := t.TempDir()
	newDir := filepath.Join(tmpHome, ".clade")

	// Run twice — should not error
	require.NoError(t, doMigrateToDotClade(tmpHome, "", "", newDir))
	require.NoError(t, doMigrateToDotClade(tmpHome, "", "", newDir))

	assert.DirExists(t, newDir)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestMigrateToDotClade`

Expected: FAIL — `doMigrateToDotClade` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/cmd/migrate_dotclade.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate Clade data to ~/.clade/ (one-time)",
	Long: `Consolidates all Clade data from ~/clade/ and ~/.config/clade/ into ~/.clade/.

This is a one-time migration for v0.8. Old paths get symlinks for backward compatibility.

Safe to run multiple times — skips files that already exist at the destination.`,
	RunE: runMigrate,
}

var migrateToDotCladeFlag bool

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().BoolVar(&migrateToDotCladeFlag, "to-dotclade", true, "Migrate to ~/.clade/ layout")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	oldConfigDir := filepath.Join(homeDir, ".config", "clade")
	oldBaseDir := filepath.Join(homeDir, "clade")
	newDir := filepath.Join(homeDir, ".clade")

	ui.Header("Migrating to ~/.clade/")
	fmt.Println()

	if err := doMigrateToDotClade(homeDir, oldConfigDir, oldBaseDir, newDir); err != nil {
		return err
	}

	ui.Success("Migration complete!")
	ui.Detail("All data now in ~/.clade/")
	ui.Detail("Old paths have symlinks for backward compatibility")
	ui.Detail("Run 'clade setup --force' to update hooks")

	return nil
}

// doMigrateToDotClade performs the actual migration. Extracted for testability.
func doMigrateToDotClade(homeDir, oldConfigDir, oldBaseDir, newDir string) error {
	// 1. Create ~/.clade/ and required subdirectories
	dirs := []string{
		newDir,
		filepath.Join(newDir, "sessions"),
		filepath.Join(newDir, "sessions", "archive"),
		filepath.Join(newDir, "inbox"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	// 2. Move files from ~/.config/clade/ to ~/.clade/
	if oldConfigDir != "" {
		configFiles := []string{"config.json", "state.json", "hooks.yaml", "trusted-repos.json"}
		for _, name := range configFiles {
			src := filepath.Join(oldConfigDir, name)
			dst := filepath.Join(newDir, name)
			migrateFile(src, dst)
		}

		// Move batches/ directory
		migrateDir(filepath.Join(oldConfigDir, "batches"), filepath.Join(newDir, "batches"))
	}

	// 3. Move repos/ from ~/clade/ to ~/.clade/repos/
	if oldBaseDir != "" {
		migrateDir(filepath.Join(oldBaseDir, "repos"), filepath.Join(newDir, "repos"))
		// Also move state.json if it was in old base dir
		migrateFile(filepath.Join(oldBaseDir, "state.json"), filepath.Join(newDir, "state.json"))
	}

	return nil
}

// migrateFile moves src to dst, then creates a symlink at src pointing to dst.
// Skips if src doesn't exist or dst already exists.
func migrateFile(src, dst string) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return // source doesn't exist
	}

	// Check if src is already a symlink
	if fi, err := os.Lstat(src); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return // already migrated
	}

	if _, err := os.Stat(dst); err == nil {
		// Destination already exists — just create symlink at source
		os.Remove(src)
		os.Symlink(dst, src)
		return
	}

	// Ensure destination directory exists
	os.MkdirAll(filepath.Dir(dst), 0755)

	// Copy file (don't use Rename across potential filesystem boundaries)
	data, err := os.ReadFile(src)
	if err != nil {
		return
	}

	// Preserve original permissions
	info, err := os.Stat(src)
	if err != nil {
		return
	}

	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return
	}

	// Replace source with symlink
	os.Remove(src)
	os.Symlink(dst, src)
}

// migrateDir moves a directory from src to dst.
// Skips if src doesn't exist or dst already exists.
func migrateDir(src, dst string) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	}

	// Check if src is already a symlink
	if fi, err := os.Lstat(src); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return
	}

	if _, err := os.Stat(dst); err == nil {
		// Destination exists — just symlink
		os.RemoveAll(src)
		os.Symlink(dst, src)
		return
	}

	// Move directory
	os.MkdirAll(filepath.Dir(dst), 0755)
	if err := os.Rename(src, dst); err != nil {
		// Rename failed (cross-device) — fall back to leaving in place
		return
	}

	// Create symlink at old location
	os.Symlink(dst, src)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kozak/kozak-labs/clade && go test ./internal/cmd/ -run TestMigrateToDotClade`

- [ ] **Step 5: Commit**

---

### Task 13: Register All New Commands in root.go

**Files:**
- Modify: `internal/cmd/root.go`

This task is a verification step. All commands are already registered via their `init()` functions (cobra convention used throughout the codebase). Each new file has:

```go
func init() {
    rootCmd.AddCommand(xxxCmd)
}
```

- [ ] **Step 1: Verify all commands are registered**

Run: `cd /home/kozak/kozak-labs/clade && go build ./cmd/clade && ./clade --help`

Verify the output includes `sessions` and `migrate` in the visible commands list. The hidden commands (`session-start`, `session-stop`, `session-stop-async`, `session-compact`) should not appear in help but should be callable.

- [ ] **Step 2: Verify hidden commands work**

Run: `cd /home/kozak/kozak-labs/clade && ./clade session-start --help`

Expected: Shows the help text for session-start (hidden but functional).

- [ ] **Step 3: Run full test suite**

Run: `cd /home/kozak/kozak-labs/clade && go test ./...`

- [ ] **Step 4: Commit**

---

### Task 14: /sessions Skill

**Files:**
- Create: `.claude/commands/sessions.md`

- [ ] **Step 1: Write the skill file**

Create `.claude/commands/sessions.md`:

```markdown
Show all active Claude Code sessions and offer management actions.

1. Run the sessions dashboard command:
   ```bash
   clade sessions --json
   ```

2. Parse the JSON output and present it in a readable format:
   - For each session, show: status, project name, age, and summary
   - Color-code: active = working now, idle = paused recently, stale = forgotten
   - Show total counts

3. Offer actions:
   - **Resume**: "Want me to resume session N?" → Run `clade sessions` interactively, or provide the `claude --resume <session_id>` command
   - **Clean**: "Want me to archive stale sessions?" → Run `clade sessions --clean`
   - **Details**: If asked about a specific session, read its dropbag from `~/.clade/sessions/<session_id>.md`

4. If no sessions exist, explain that sessions are tracked automatically via hooks and suggest running `clade setup --force` if hooks aren't configured.

Example output format:

```
Found 3 sessions:

1. [active] leap-complete (2h) — API gateway debug, posted instructions
2. [idle 4h] document-engine — Lambda fix DONE, needs PR
3. [stale 1d] chezmoi — Dotfiles tmux config

What would you like to do?
```
```

- [ ] **Step 2: Verify the file is accessible**

Run: `cat /home/kozak/kozak-labs/clade/.claude/commands/sessions.md`

- [ ] **Step 3: Commit**

---

### Task 15: /drop Skill Update

**Files:**
- Modify: `.claude/commands/drop.md`
- Modify: `internal/cmd/init.go` (update `dropCommandContent` constant)

- [ ] **Step 1: Update the drop command to write to centralized locations**

Update `.claude/commands/drop.md` (and the `dropCommandContent` constant in `internal/cmd/init.go`) to:

```markdown
Create a session summary that saves to TWO places for cross-session awareness:

## Step 1: Determine session ID and today's date

```bash
# Get today's date for inbox
TODAY=$(date +%Y-%m-%d)
TIMESTAMP=$(date +%Y-%m-%d-%H%M)
TIME_DISPLAY=$(date +"%l:%M %p" | sed 's/^ //')
```

## Step 2: Write session dropbag to ~/.clade/sessions/

If the CLAUDE_SESSION_ID env var is available, write to `~/.clade/sessions/$CLAUDE_SESSION_ID.md`.
Otherwise, write to the legacy location `.clade/dropbags/DROPBAG-$TIMESTAMP.md`.

```bash
mkdir -p ~/.clade/sessions
cat > ~/.clade/sessions/${CLAUDE_SESSION_ID:-dropbag-$TIMESTAMP}.md <<'EOF'
[your content here]
EOF
```

## Step 3: Append to daily inbox for cross-session broadcast

```bash
mkdir -p ~/.clade/inbox
cat >> ~/.clade/inbox/$TODAY.md <<EOF

### $TIME_DISPLAY | $PROJECT_NAME | handoff
[One-line summary of what was accomplished and what's next]
EOF
```

## Content format for the dropbag:

### Summary
What we accomplished this session. Be specific about changes made.

### Current State
What's working, what's broken, what's partially implemented.

### Next Steps
Exact actions to continue (be specific - file names, function names, etc.).

### Key Files
Files to look at first when resuming. Include line numbers if relevant.

### Open Questions
Anything unresolved or decisions that need to be made.

---

After saving both files, confirm:
1. The session dropbag was created in ~/.clade/sessions/
2. The inbox entry was appended to ~/.clade/inbox/$TODAY.md
```

- [ ] **Step 2: Update the dropCommandContent constant in init.go**

Replace the `dropCommandContent` constant in `internal/cmd/init.go` with the content above.

- [ ] **Step 3: Verify the file content matches**

Run: `cd /home/kozak/kozak-labs/clade && go build ./cmd/clade`

- [ ] **Step 4: Commit**

---

## Post-Implementation Checklist

- [ ] All tests pass: `cd /home/kozak/kozak-labs/clade && go test ./...`
- [ ] Build succeeds: `cd /home/kozak/kozak-labs/clade && go build ./cmd/clade`
- [ ] `clade sessions` shows dashboard (manual test)
- [ ] `clade migrate --to-dotclade` creates ~/.clade/ structure (manual test)
- [ ] `clade setup --force` registers new hooks (manual test)
- [ ] Open 2 Claude sessions, verify `clade sessions` shows both (manual test)
- [ ] Stop a session, verify inbox entry created (manual test)
- [ ] Start new session, verify inbox context injected (manual test)

## Dependency Graph

```
Task 1 (types) ──┬──> Task 2 (registry) ──┬──> Task 4 (session-start)
                  │                        ├──> Task 5 (session-stop)
                  │                        ├──> Task 7 (session-compact)
                  │                        └──> Task 8 (sessions dashboard)
                  │
                  └──> Task 3 (inbox) ─────┬──> Task 5 (session-stop)
                                           ├──> Task 6 (session-stop-async)
                                           └──> Task 9 (inject inbox scan)

Task 5 (session-stop) ──> Task 6 (session-stop-async)

Task 11 (config paths) ──> Task 12 (migration)

Task 4-8 ──> Task 10 (setup hooks)
Task 4-8 ──> Task 13 (register commands)

Task 13 ──> Task 14 (/sessions skill)
Task 13 ──> Task 15 (/drop update)
```

Tasks 1-3 can be parallelized. Tasks 4, 5, 7 can be parallelized after 1-3. Task 6 depends on 5. Tasks 8-15 depend on earlier tasks as shown.
