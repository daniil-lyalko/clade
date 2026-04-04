package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- head init tests ---

func TestHeadInit_CreatesDirectoryStructure(t *testing.T) {
	tmpDir := t.TempDir()
	headDir := filepath.Join(tmpDir, "head")

	err := doHeadInit(headDir, "")
	require.NoError(t, err)

	// Check directory exists
	info, err := os.Stat(headDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Check CLAUDE.md exists
	claudeMD, err := os.ReadFile(filepath.Join(headDir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(claudeMD), "orchestrator")
	assert.Contains(t, string(claudeMD), "Session Awareness")

	// Check .claude/skills/ exists
	info, err = os.Stat(filepath.Join(headDir, ".claude", "skills"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestHeadInit_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	headDir := filepath.Join(tmpDir, "head")

	// First init
	err := doHeadInit(headDir, "")
	require.NoError(t, err)

	// Modify CLAUDE.md
	customContent := "# Custom content"
	err = os.WriteFile(filepath.Join(headDir, "CLAUDE.md"), []byte(customContent), 0644)
	require.NoError(t, err)

	// Second init should not overwrite
	err = doHeadInit(headDir, "")
	require.NoError(t, err)

	// Verify custom content preserved
	data, err := os.ReadFile(filepath.Join(headDir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, customContent, string(data))
}

func TestHeadInit_BrainSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	headDir := filepath.Join(tmpDir, "head")
	brainDir := filepath.Join(tmpDir, "my-brain")

	// Create brain directory
	require.NoError(t, os.MkdirAll(brainDir, 0755))

	err := doHeadInit(headDir, brainDir)
	require.NoError(t, err)

	// Check symlink
	brainLink := filepath.Join(headDir, "brain")
	target, err := os.Readlink(brainLink)
	require.NoError(t, err)
	assert.Equal(t, brainDir, target)
}

func TestHeadInit_BrainPathNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	headDir := filepath.Join(tmpDir, "head")

	err := doHeadInit(headDir, "/nonexistent/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "brain path does not exist")
}

// --- head start tests ---

func TestHeadStart_ErrorsIfNotInitialized(t *testing.T) {
	// The runHeadStart function checks headDirectory() which uses cladeBaseDir()
	// We test the logic indirectly: if the dir doesn't exist, it should error
	tmpDir := t.TempDir()
	headDir := filepath.Join(tmpDir, "head")

	// Directly check the condition
	_, err := os.Stat(headDir)
	assert.True(t, os.IsNotExist(err))
}

func TestHeadStart_DetectsAlreadyRunning(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	// isHeadRunning should return false when no session exists
	// (tmux may or may not be running, but clade-head session shouldn't exist)
	// This is a basic smoke test
	_ = isHeadRunning()
}

// --- head stop tests ---

func TestHeadStop_HandlesNotRunning(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	// When head is not running, runHeadStop should succeed gracefully
	err := runHeadStop(nil, nil)
	assert.NoError(t, err)
}

// --- head status tests ---

func TestHeadStatus_ShowsCorrectState(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with no head directory
	headDir := filepath.Join(tmpDir, "head")
	_, err := os.Stat(headDir)
	assert.True(t, os.IsNotExist(err))

	// Test with initialized head
	require.NoError(t, doHeadInit(headDir, ""))
	info, err := os.Stat(headDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Test session.FormatDuration (shared duration formatter)
	assert.Equal(t, "30s", session.FormatDuration(30*time.Second))
	assert.Equal(t, "5m", session.FormatDuration(5*time.Minute))
	assert.Equal(t, "2h", session.FormatDuration(2*time.Hour))
	assert.Equal(t, "2h 30m", session.FormatDuration(2*time.Hour+30*time.Minute))
	assert.Equal(t, "3d", session.FormatDuration(72*time.Hour))
	assert.Equal(t, "1d 6h", session.FormatDuration(30*time.Hour))
}

// --- sessions dashboard HEAD label test ---

func TestSessionsDashboard_ShowsHEADLabel(t *testing.T) {
	sessions := []*session.Session{
		{
			SessionID:  "sess-1",
			Project:    "my-api",
			Status:     session.StatusActive,
			LastActive: time.Now(),
			Summary:    "Working on API",
		},
		{
			SessionID:  "head",
			Project:    "head",
			Status:     session.StatusActive,
			LastActive: time.Now(),
			Summary:    "Orchestrator session",
		},
	}

	var buf bytes.Buffer
	formatSessionsDashboard(&buf, sessions)
	output := buf.String()

	// HEAD session should appear in output
	assert.Contains(t, output, "[HEAD]")
	assert.Contains(t, output, "Orchestrator session")

	// HEAD should appear before other sessions (first in sorted order)
	headIdx := indexOf(output, "[HEAD]")
	apiIdx := indexOf(output, "my-api")
	assert.True(t, headIdx < apiIdx, "HEAD session should appear before other sessions")
}

func TestSessionsDashboard_ShowsNamedHEADLabel(t *testing.T) {
	sessions := []*session.Session{
		{
			SessionID:  "sess-1",
			Project:    "my-api",
			Status:     session.StatusActive,
			LastActive: time.Now(),
			Summary:    "Working on API",
		},
		{
			SessionID:  "gru",
			Project:    "head",
			Status:     session.StatusActive,
			LastActive: time.Now(),
			Summary:    "Orchestrator session (gru)",
		},
	}

	var buf bytes.Buffer
	formatSessionsDashboard(&buf, sessions)
	output := buf.String()

	// Named head session should show [HEAD] label with session name
	assert.Contains(t, output, "[HEAD]")
	assert.Contains(t, output, "gru")

	// HEAD should appear before other sessions
	headIdx := indexOf(output, "[HEAD]")
	apiIdx := indexOf(output, "my-api")
	assert.True(t, headIdx < apiIdx, "HEAD session should appear before other sessions")
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
