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
