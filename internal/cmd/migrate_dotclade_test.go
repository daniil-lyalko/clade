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
