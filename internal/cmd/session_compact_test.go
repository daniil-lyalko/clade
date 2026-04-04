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
