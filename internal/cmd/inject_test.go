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
	dropbagPath, err := reg.DropbagPath("test-session")
	require.NoError(t, err)
	data, err := os.ReadFile(dropbagPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Some context here")
}
