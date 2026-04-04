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
