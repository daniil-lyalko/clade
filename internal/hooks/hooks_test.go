package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadHooksConfig_ValidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	hooksFile := filepath.Join(tmpDir, "hooks.yaml")

	content := `hooks:
  on_create:
    - npm install
    - cp .env.example .env
  on_resume:
    - direnv allow
  on_remove:
    - echo "cleanup"
`
	require.NoError(t, os.WriteFile(hooksFile, []byte(content), 0644))

	config, err := loadHooksConfig(hooksFile)

	require.NoError(t, err)
	assert.Len(t, config.Hooks[OnCreate], 2)
	assert.Equal(t, "npm install", config.Hooks[OnCreate][0])
	assert.Equal(t, "cp .env.example .env", config.Hooks[OnCreate][1])
	assert.Len(t, config.Hooks[OnResume], 1)
	assert.Equal(t, "direnv allow", config.Hooks[OnResume][0])
}

func TestLoadHooksConfig_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	hooksFile := filepath.Join(tmpDir, "hooks.yaml")

	require.NoError(t, os.WriteFile(hooksFile, []byte(""), 0644))

	config, err := loadHooksConfig(hooksFile)

	require.NoError(t, err)
	assert.NotNil(t, config.Hooks)
}

func TestLoadHooksConfig_FileNotFound(t *testing.T) {
	config, err := loadHooksConfig("/nonexistent/hooks.yaml")

	// Should not error, just return empty config
	require.NoError(t, err)
	assert.NotNil(t, config.Hooks)
	assert.Empty(t, config.Hooks)
}

func TestLoadHooksConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	hooksFile := filepath.Join(tmpDir, "hooks.yaml")

	require.NoError(t, os.WriteFile(hooksFile, []byte("invalid: yaml: content: ["), 0644))

	_, err := loadHooksConfig(hooksFile)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestEnv_ToEnvVars(t *testing.T) {
	env := &Env{
		Type:     "spike",
		Name:     "try-redis",
		Path:     "/path/to/worktree",
		RepoName: "my-api",
		RepoPath: "/path/to/repo",
		Branch:   "spike/try-redis",
		Ticket:   "PROJ-1234",
	}

	vars := env.toEnvVars()

	assert.Contains(t, vars, "PACER_TYPE=spike")
	assert.Contains(t, vars, "PACER_NAME=try-redis")
	assert.Contains(t, vars, "PACER_PATH=/path/to/worktree")
	assert.Contains(t, vars, "PACER_REPO_NAME=my-api")
	assert.Contains(t, vars, "PACER_REPO_PATH=/path/to/repo")
	assert.Contains(t, vars, "PACER_BRANCH=spike/try-redis")
	assert.Contains(t, vars, "PACER_TICKET=PROJ-1234")
}

func TestEnv_ToEnvVars_NoTicket(t *testing.T) {
	env := &Env{
		Type:     "feature",
		Name:     "new-api",
		Path:     "/path/to/worktree",
		RepoName: "my-api",
		RepoPath: "/path/to/repo",
		Branch:   "feat/new-api",
		// No ticket
	}

	vars := env.toEnvVars()

	// Should not include PACER_TICKET
	for _, v := range vars {
		assert.NotContains(t, v, "PACER_TICKET=")
	}
}

func TestRunHooks_SimpleCommand(t *testing.T) {
	tmpDir := t.TempDir()

	// Create global hooks
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "pacer")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", originalHome)

	hooksFile := filepath.Join(configDir, "hooks.yaml")
	content := `hooks:
  on_create:
    - echo "test output"
`
	require.NoError(t, os.WriteFile(hooksFile, []byte(content), 0644))

	env := &Env{
		Type:     "spike",
		Name:     "test",
		Path:     tmpDir,
		RepoName: "test-repo",
		RepoPath: tmpDir,
		Branch:   "spike/test",
	}

	results := RunHooks(OnCreate, env)

	assert.Len(t, results, 1)
	assert.Equal(t, `echo "test output"`, results[0].Command)
	assert.Contains(t, results[0].Output, "test output")
	assert.NoError(t, results[0].Error)
	assert.Greater(t, results[0].Duration, 0*time.Nanosecond)
}

func TestRunHooks_MultipleCommands(t *testing.T) {
	tmpDir := t.TempDir()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "pacer")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", originalHome)

	hooksFile := filepath.Join(configDir, "hooks.yaml")
	content := `hooks:
  on_create:
    - echo "first"
    - echo "second"
    - echo "third"
`
	require.NoError(t, os.WriteFile(hooksFile, []byte(content), 0644))

	env := &Env{
		Path:     tmpDir,
		RepoName: "test",
		RepoPath: tmpDir,
	}

	results := RunHooks(OnCreate, env)

	assert.Len(t, results, 3)
	assert.Contains(t, results[0].Output, "first")
	assert.Contains(t, results[1].Output, "second")
	assert.Contains(t, results[2].Output, "third")
}

func TestRunHooks_FailureContinues(t *testing.T) {
	// Verify that failed hooks don't stop subsequent hooks

	tmpDir := t.TempDir()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "pacer")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", originalHome)

	hooksFile := filepath.Join(configDir, "hooks.yaml")
	content := `hooks:
  on_create:
    - echo "before"
    - false
    - echo "after"
`
	require.NoError(t, os.WriteFile(hooksFile, []byte(content), 0644))

	env := &Env{
		Path:     tmpDir,
		RepoName: "test",
		RepoPath: tmpDir,
	}

	results := RunHooks(OnCreate, env)

	// All three hooks should have run
	assert.Len(t, results, 3)
	assert.NoError(t, results[0].Error)
	assert.Error(t, results[1].Error) // false command fails
	assert.NoError(t, results[2].Error)
}

func TestRunHooks_EnvironmentVariables(t *testing.T) {
	tmpDir := t.TempDir()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "pacer")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", originalHome)

	hooksFile := filepath.Join(configDir, "hooks.yaml")
	content := `hooks:
  on_create:
    - echo $PACER_NAME
    - echo $PACER_TYPE
    - echo $PACER_BRANCH
`
	require.NoError(t, os.WriteFile(hooksFile, []byte(content), 0644))

	env := &Env{
		Type:     "spike",
		Name:     "test-exp",
		Path:     tmpDir,
		RepoName: "my-repo",
		RepoPath: tmpDir,
		Branch:   "spike/test-exp",
	}

	results := RunHooks(OnCreate, env)

	require.Len(t, results, 3)
	assert.Contains(t, results[0].Output, "test-exp")
	assert.Contains(t, results[1].Output, "spike")
	assert.Contains(t, results[2].Output, "spike/test-exp")
}

func TestRunHooks_RepoSpecificHooks(t *testing.T) {
	// Test that repo-specific hooks run after global hooks

	tmpDir := t.TempDir()

	// Setup global hooks
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "pacer")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", originalHome)

	// Trust repo hooks for testing
	os.Setenv("PACER_TRUST_REPO_HOOKS", "1")
	defer os.Unsetenv("PACER_TRUST_REPO_HOOKS")

	globalHooks := filepath.Join(configDir, "hooks.yaml")
	globalContent := `hooks:
  on_create:
    - echo "global"
`
	require.NoError(t, os.WriteFile(globalHooks, []byte(globalContent), 0644))

	// Setup repo-specific hooks
	repoHooksDir := filepath.Join(tmpDir, ".pacer")
	require.NoError(t, os.MkdirAll(repoHooksDir, 0755))

	repoHooks := filepath.Join(repoHooksDir, "hooks.yaml")
	repoContent := `hooks:
  on_create:
    - echo "repo-specific"
`
	require.NoError(t, os.WriteFile(repoHooks, []byte(repoContent), 0644))

	env := &Env{
		Path:     tmpDir,
		RepoName: "test",
		RepoPath: tmpDir,
	}

	results := RunHooks(OnCreate, env)

	// Should run both global and repo-specific hooks
	assert.Len(t, results, 2)
	assert.Contains(t, results[0].Output, "global")
	assert.Contains(t, results[1].Output, "repo-specific")
}

func TestHasHooks_GlobalOnly(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "pacer")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", originalHome)

	hooksFile := filepath.Join(configDir, "hooks.yaml")
	content := `hooks:
  on_create:
    - echo "test"
`
	require.NoError(t, os.WriteFile(hooksFile, []byte(content), 0644))

	assert.True(t, HasHooks(OnCreate, ""))
	assert.False(t, HasHooks(OnResume, ""))
}

func TestHasHooks_RepoOnly(t *testing.T) {
	tmpDir := t.TempDir()
	repoHooksDir := filepath.Join(tmpDir, ".pacer")
	require.NoError(t, os.MkdirAll(repoHooksDir, 0755))

	hooksFile := filepath.Join(repoHooksDir, "hooks.yaml")
	content := `hooks:
  on_resume:
    - echo "test"
`
	require.NoError(t, os.WriteFile(hooksFile, []byte(content), 0644))

	assert.True(t, HasHooks(OnResume, tmpDir))
	assert.False(t, HasHooks(OnCreate, tmpDir))
}

func TestHasHooks_NoHooks(t *testing.T) {
	assert.False(t, HasHooks(OnCreate, ""))
	assert.False(t, HasHooks(OnResume, "/nonexistent"))
}
