package session

import (
	"fmt"
	"time"
)

type SessionStatus string

const (
	StatusActive   SessionStatus = "active"
	StatusStopped  SessionStatus = "stopped"
	StatusStopping SessionStatus = "stopping"
)

// HeadSessionID is the canonical session ID / project name used for the
// persistent orchestrator ("head") session.
const HeadSessionID = "head"

type Session struct {
	SessionID       string        `json:"session_id"`
	Project         string        `json:"project"`
	CWD             string        `json:"cwd"`
	Branch          string        `json:"branch"`
	Started         time.Time     `json:"started"`
	LastActive      time.Time     `json:"last_active"`
	Status          SessionStatus `json:"status"`
	Summary         string        `json:"summary,omitempty"`
	IsWorktree      bool          `json:"is_worktree"`
	WorktreeName    string        `json:"worktree_name,omitempty"`
	TokensUsed      int64         `json:"tokens_used,omitempty"`
	InboxReadOffset int64         `json:"inbox_read_offset,omitempty"`
}

const (
	StaleThreshold   = 24 * time.Hour
	ArchiveThreshold = 7 * 24 * time.Hour
	StoppingTimeout  = 5 * time.Minute
)

func (s *Session) IsStale() bool {
	return time.Since(s.LastActive) > StaleThreshold
}

func (s *Session) IsArchivable() bool {
	return time.Since(s.LastActive) > ArchiveThreshold
}

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
			return fmt.Sprintf("stale %s", FormatDurationCompact(age))
		}
		return fmt.Sprintf("idle %s", FormatDurationCompact(age))
	default:
		return string(s.Status)
	}
}

// FormatDuration formats a duration in a human-readable compact form.
// Examples: "5s", "12m", "2h 30m", "3d 2h".
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	if h > 0 {
		return fmt.Sprintf("%dd %dh", days, h)
	}
	return fmt.Sprintf("%dd", days)
}

// FormatDurationCompact is like FormatDuration but always returns a single
// unit (e.g. "2h" instead of "2h 30m"). Used for tight display contexts.
func FormatDurationCompact(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

type InboxEntryType string

const (
	EntryDecision InboxEntryType = "decision"
	EntryFYI      InboxEntryType = "fyi"
	EntryBlocker  InboxEntryType = "blocker"
	EntryHandoff  InboxEntryType = "handoff"
)

func (t InboxEntryType) IsValid() bool {
	switch t {
	case EntryDecision, EntryFYI, EntryBlocker, EntryHandoff:
		return true
	}
	return false
}

type InboxEntry struct {
	Time      time.Time      `json:"-"`
	Project   string         `json:"-"`
	EntryType InboxEntryType `json:"-"`
	Message   string         `json:"-"`
	SessionID string         `json:"-"`
}
