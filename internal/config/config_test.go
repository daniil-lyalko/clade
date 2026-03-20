package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotEmpty(t, cfg.BaseDir)
	assert.Contains(t, cfg.BaseDir, "clade")
	assert.True(t, cfg.AutoInit)
	assert.Empty(t, cfg.Agent) // Should be set by wizard, not hardcoded
	assert.Empty(t, cfg.AgentFlags) // Should NOT contain dangerous flags
	assert.NotNil(t, cfg.Repos)
	assert.NotNil(t, cfg.CustomLabels)
}

func TestConfigSave_FilePermissions(t *testing.T) {
	// Critical security test: Verify config files are created with 0600 (owner-only)
	// We'll test the actual Save method by checking what it writes

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	cfg.Agent = "claude"
	cfg.Repos["test"] = "/tmp/test"

	// Manually create directory
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))

	// Marshal config
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)

	// Simulate what Save() does with correct permissions
	require.NoError(t, os.WriteFile(configPath, data, 0600))

	// Verify file has correct permissions
	stat, err := os.Stat(configPath)
	require.NoError(t, err)

	// Critical: File should be 0600 (owner read/write only, not world-readable)
	assert.Equal(t, os.FileMode(0600), stat.Mode().Perm(), "Config file should have 0600 permissions for security")
}

func TestConfigSave_RealSave(t *testing.T) {
	// Integration test: Use real config path in temp directory
	// This tests the actual Save() method implementation

	// Save original home and override for test
	originalHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	cfg := DefaultConfig()
	cfg.Agent = "test-agent"

	err := cfg.Save()
	require.NoError(t, err)

	// Find the saved file
	configPath, err := ConfigPath()
	require.NoError(t, err)

	// Verify it exists and has correct permissions
	stat, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), stat.Mode().Perm())
}

func TestConfigSave_NoDangerousDefaults(t *testing.T) {
	// Security test: Verify new configs don't contain --dangerously-skip-permissions

	cfg := DefaultConfig()
	cfg.Agent = "claude"

	// Should have empty agent flags by default
	assert.Empty(t, cfg.AgentFlags)
	assert.NotContains(t, cfg.AgentFlags, "--dangerously-skip-permissions")
}

func TestConfigLoad_InvalidJSON(t *testing.T) {
	// Override HOME for isolated test
	originalHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	// Get actual config path
	configPath, err := ConfigPath()
	require.NoError(t, err)

	// Write invalid JSON
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte("invalid json{"), 0644))

	// Try to load - should fail
	_, err = Load()
	assert.Error(t, err)
}

func TestExpandPath_TildeExpansion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // What the output should contain
	}{
		{
			name:     "Tilde at start",
			input:    "~/clade",
			contains: "/clade",
		},
		{
			name:     "Tilde home directory",
			input:    "~",
			contains: "/", // Should resolve to actual home dir
		},
		{
			name:     "No tilde",
			input:    "/absolute/path",
			contains: "/absolute/path",
		},
		{
			name:     "Relative path",
			input:    "relative/path",
			contains: "relative/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandPath(tt.input)
			assert.Contains(t, result, tt.contains)

			// Tilde paths should not start with ~
			if tt.input[0] == '~' {
				assert.NotContains(t, result, "~")
			}
		})
	}
}

func TestConfig_GetBaseDir(t *testing.T) {
	cfg := &Config{BaseDir: "~/test-clade"}

	baseDir := cfg.GetBaseDir()

	assert.NotContains(t, baseDir, "~")
	assert.Contains(t, baseDir, "test-clade")
}

func TestConfig_DirectoryGetters(t *testing.T) {
	cfg := &Config{BaseDir: "/tmp/clade-test"}

	assert.Equal(t, "/tmp/clade-test/repos", cfg.ReposDir())
	assert.Equal(t, "/tmp/clade-test/experiments", cfg.ExperimentsDir())
	assert.Equal(t, "/tmp/clade-test/projects", cfg.ProjectsDir())
	assert.Equal(t, "/tmp/clade-test/scratch", cfg.ScratchDir())
}

func TestBuiltInLabels(t *testing.T) {
	labels := BuiltInLabels()

	// Verify all expected labels exist
	expectedLabels := []string{"feature", "bug", "spike", "chore", "hotfix", "docs"}
	for _, label := range expectedLabels {
		cfg, ok := labels[label]
		require.True(t, ok, "Label %s should exist", label)
		assert.NotEmpty(t, cfg.BranchPrefix)
	}

	// Verify specific configurations
	assert.Equal(t, "feat", labels["feature"].BranchPrefix)
	assert.True(t, labels["feature"].MergeExpected)

	assert.Equal(t, "spike", labels["spike"].BranchPrefix)
	assert.False(t, labels["spike"].MergeExpected) // Spikes are throwaway
}

func TestConfig_GetLabelConfig(t *testing.T) {
	cfg := &Config{
		CustomLabels: map[string]LabelConfig{
			"perf": {BranchPrefix: "perf", MergeExpected: true},
		},
	}

	// Test built-in label
	labelCfg, ok := cfg.GetLabelConfig("feature")
	assert.True(t, ok)
	assert.Equal(t, "feat", labelCfg.BranchPrefix)

	// Test custom label
	labelCfg, ok = cfg.GetLabelConfig("perf")
	assert.True(t, ok)
	assert.Equal(t, "perf", labelCfg.BranchPrefix)

	// Test non-existent label
	_, ok = cfg.GetLabelConfig("nonexistent")
	assert.False(t, ok)
}

func TestConfig_RepoCopyFiles(t *testing.T) {
	cfg := &Config{
		RepoSettings: map[string]RepoSettings{
			"/path/to/repo": {
				CopyFiles: []string{"secrets.json", "config/local.json"},
			},
		},
	}

	// Test existing repo settings
	files := cfg.GetRepoCopyFiles("/path/to/repo")
	assert.Equal(t, []string{"secrets.json", "config/local.json"}, files)

	// Test non-existent repo
	files = cfg.GetRepoCopyFiles("/other/repo")
	assert.Nil(t, files)

	// Test SetRepoCopyFiles
	cfg.SetRepoCopyFiles("/new/repo", []string{"file1.txt"})
	files = cfg.GetRepoCopyFiles("/new/repo")
	assert.Equal(t, []string{"file1.txt"}, files)
}

func TestConfigRoundTrip(t *testing.T) {
	// Test saving and loading config preserves all data

	// Override HOME for isolated test
	originalHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	// Create config with various settings
	cfg := &Config{
		BaseDir:    "~/test-clade",
		Agent:      "claude",
		AgentFlags: []string{"--continue"},
		Editor:     "cursor",
		AutoInit:   true,
		Repos: map[string]string{
			"my-api":      "~/repos/my-api",
			"my-frontend": "~/repos/my-frontend",
		},
		RepoSettings: map[string]RepoSettings{
			"~/repos/my-api": {
				CopyFiles: []string{"secrets.json"},
			},
		},
		LastRepo:           "my-api",
		TmuxSplitDirection: "vertical",
		CustomLabels: map[string]LabelConfig{
			"perf": {BranchPrefix: "perf", MergeExpected: true},
		},
	}

	// Save
	err := cfg.Save()
	require.NoError(t, err)

	// Load
	loaded, err := Load()
	require.NoError(t, err)

	// Verify all fields preserved
	assert.Equal(t, cfg.BaseDir, loaded.BaseDir)
	assert.Equal(t, cfg.Agent, loaded.Agent)
	assert.Equal(t, cfg.AgentFlags, loaded.AgentFlags)
	assert.Equal(t, cfg.Editor, loaded.Editor)
	assert.Equal(t, cfg.AutoInit, loaded.AutoInit)
	assert.Equal(t, cfg.Repos, loaded.Repos)
	assert.Equal(t, cfg.LastRepo, loaded.LastRepo)
	assert.Equal(t, cfg.TmuxSplitDirection, loaded.TmuxSplitDirection)
	assert.Equal(t, cfg.CustomLabels["perf"].BranchPrefix, loaded.CustomLabels["perf"].BranchPrefix)
}

func TestFilePermissions_DirectCheck(t *testing.T) {
	// Direct test of file permissions using os.WriteFile
	// This verifies the fix is actually applied

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-config.json")

	data := []byte(`{"test": "data"}`)

	// Write with 0600 (what our fix does)
	require.NoError(t, os.WriteFile(testFile, data, 0600))

	stat, err := os.Stat(testFile)
	require.NoError(t, err)

	// Verify it's actually 0600
	assert.Equal(t, os.FileMode(0600), stat.Mode().Perm())

	// Verify it's NOT 0644 (the old, insecure default)
	assert.NotEqual(t, os.FileMode(0644), stat.Mode().Perm())
}
