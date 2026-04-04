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
