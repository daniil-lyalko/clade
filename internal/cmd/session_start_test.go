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
