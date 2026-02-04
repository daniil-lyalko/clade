package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadState_EmptyState(t *testing.T) {
	tmpDir := t.TempDir()
	SetTestStateDir(tmpDir)
	defer SetTestStateDir("")

	cfg := &Config{BaseDir: tmpDir}

	state, err := LoadState(cfg)

	require.NoError(t, err)
	assert.Equal(t, 2, state.Version)
	assert.NotNil(t, state.Worktrees)
	assert.NotNil(t, state.Projects)
	assert.NotNil(t, state.Scratches)
	assert.Empty(t, state.Worktrees)
}

func TestStateSave_FilePermissions(t *testing.T) {
	// Critical security test: Verify state files are created with 0600 (owner-only)

	tmpDir := t.TempDir()
	SetTestStateDir(tmpDir)
	defer SetTestStateDir("")

	cfg := &Config{BaseDir: tmpDir}

	state := &State{
		Version:   2,
		Worktrees: make(map[string]map[string]*Worktree),
		Projects:  make(map[string]*Project),
		Scratches: make(map[string]*Scratch),
	}

	err := state.Save(cfg)
	require.NoError(t, err)

	statePath := StatePath()
	stat, err := os.Stat(statePath)
	require.NoError(t, err)

	// Critical: File should be 0600 (owner read/write only, not world-readable)
	assert.Equal(t, os.FileMode(0600), stat.Mode().Perm(), "State file should have 0600 permissions for security")
}

func TestState_AddWorktree(t *testing.T) {
	state := &State{
		Version:   2,
		Worktrees: make(map[string]map[string]*Worktree),
	}

	wt := &Worktree{
		Name:     "test-feature",
		Label:    "feature",
		Branch:   "feat/test-feature",
		Created:  time.Now(),
		LastUsed: time.Now(),
	}

	state.AddWorktree("my-repo", wt)

	// Verify worktree was added
	assert.NotNil(t, state.Worktrees["my-repo"])
	assert.Equal(t, wt, state.Worktrees["my-repo"]["test-feature"])
}

func TestState_GetWorktree(t *testing.T) {
	state := &State{
		Version:   2,
		Worktrees: make(map[string]map[string]*Worktree),
	}

	wt := &Worktree{
		Name:   "test",
		Branch: "feat/test",
	}

	state.AddWorktree("my-repo", wt)

	// Test successful get
	retrieved := state.GetWorktree("my-repo", "test")
	assert.Equal(t, wt, retrieved)

	// Test non-existent worktree
	notFound := state.GetWorktree("my-repo", "nonexistent")
	assert.Nil(t, notFound)

	// Test non-existent repo
	notFound = state.GetWorktree("other-repo", "test")
	assert.Nil(t, notFound)
}

func TestState_RemoveWorktree(t *testing.T) {
	state := &State{
		Version:   2,
		Worktrees: make(map[string]map[string]*Worktree),
	}

	// Add worktrees
	state.AddWorktree("my-repo", &Worktree{Name: "wt1", Branch: "feat/wt1"})
	state.AddWorktree("my-repo", &Worktree{Name: "wt2", Branch: "feat/wt2"})
	state.AddWorktree("other-repo", &Worktree{Name: "wt3", Branch: "feat/wt3"})

	// Remove one worktree
	state.RemoveWorktree("my-repo", "wt1")

	assert.Nil(t, state.GetWorktree("my-repo", "wt1"))
	assert.NotNil(t, state.GetWorktree("my-repo", "wt2"))
	assert.NotNil(t, state.GetWorktree("other-repo", "wt3"))

	// Remove last worktree from repo
	state.RemoveWorktree("my-repo", "wt2")

	// Should clean up empty repo map
	assert.Nil(t, state.Worktrees["my-repo"])
}

func TestState_FindWorktreeByName(t *testing.T) {
	state := &State{
		Version:   2,
		Worktrees: make(map[string]map[string]*Worktree),
	}

	wt1 := &Worktree{Name: "test", Branch: "feat/test"}
	wt2 := &Worktree{Name: "other", Branch: "fix/other"}

	state.AddWorktree("repo1", wt1)
	state.AddWorktree("repo2", wt2)

	// Find existing worktree
	repoName, found := state.FindWorktreeByName("test")
	assert.Equal(t, "repo1", repoName)
	assert.Equal(t, wt1, found)

	// Find non-existent worktree
	repoName, found = state.FindWorktreeByName("nonexistent")
	assert.Empty(t, repoName)
	assert.Nil(t, found)
}

func TestState_LegacyExperiments(t *testing.T) {
	// Test v1 format compatibility
	state := &State{
		Version:     1,
		Experiments: make(map[string]*Experiment),
	}

	exp := &Experiment{
		Name:     "test-exp",
		Repo:     "/path/to/repo",
		Path:     "/path/to/worktree",
		Branch:   "exp/test-exp",
		Created:  time.Now(),
		LastUsed: time.Now(),
	}

	state.AddExperiment(exp)

	key := ExperimentKey(exp.Repo, exp.Name)
	retrieved := state.GetExperiment(key)
	assert.Equal(t, exp, retrieved)

	// Test removal
	state.RemoveExperiment(key)
	assert.Nil(t, state.GetExperiment(key))
}

func TestExperimentKey(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		expName  string
		expected string
	}{
		{
			name:     "Basic repo path",
			repo:     "/path/to/my-api",
			expName:  "try-redis",
			expected: "my-api-try-redis",
		},
		{
			name:     "Repo path with trailing slash",
			repo:     "/path/to/frontend/",
			expName:  "ui-redesign",
			expected: "frontend-ui-redesign",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := ExperimentKey(tt.repo, tt.expName)
			assert.Equal(t, tt.expected, key)
		})
	}
}

func TestState_Scratches(t *testing.T) {
	state := &State{
		Version:   2,
		Scratches: make(map[string]*Scratch),
	}

	scratch := &Scratch{
		Name:     "doc-review",
		Path:     "/path/to/scratch",
		Created:  time.Now(),
		LastUsed: time.Now(),
	}

	// Add
	state.AddScratch(scratch)
	assert.Equal(t, scratch, state.GetScratch("doc-review"))

	// Update (add with same name)
	updated := &Scratch{
		Name:     "doc-review",
		Path:     "/different/path",
		Created:  scratch.Created,
		LastUsed: time.Now(),
	}
	state.AddScratch(updated)
	assert.Equal(t, updated, state.GetScratch("doc-review"))

	// Remove
	state.RemoveScratch("doc-review")
	assert.Nil(t, state.GetScratch("doc-review"))
}

func TestStateRoundTrip(t *testing.T) {
	// Test saving and loading state preserves all data

	tmpDir := t.TempDir()
	SetTestStateDir(tmpDir)
	defer SetTestStateDir("")

	cfg := &Config{BaseDir: tmpDir}

	now := time.Now()
	state := &State{
		Version: 2,
		Worktrees: map[string]map[string]*Worktree{
			"repo1": {
				"wt1": {
					Name:     "wt1",
					Label:    "feature",
					Branch:   "feat/wt1",
					Ticket:   "PROJ-123",
					Created:  now,
					LastUsed: now,
				},
			},
		},
		Experiments: map[string]*Experiment{
			"repo-exp1": {
				Name:     "exp1",
				Repo:     "/path/to/repo",
				Path:     "/path/to/exp",
				Branch:   "exp/exp1",
				Created:  now,
				LastUsed: now,
			},
		},
		Projects: map[string]*Project{
			"proj1": {
				Name:   "proj1",
				Path:   "/path/to/project",
				Branch: "feat/proj1",
				Repos: []ProjectRepo{
					{Name: "backend", Source: "/repo1"},
				},
				Created:  now,
				LastUsed: now,
			},
		},
		Scratches: map[string]*Scratch{
			"scratch1": {
				Name:     "scratch1",
				Path:     "/path/to/scratch",
				Created:  now,
				LastUsed: now,
			},
		},
	}

	// Save
	err := state.Save(cfg)
	require.NoError(t, err)

	// Load
	loaded, err := LoadState(cfg)
	require.NoError(t, err)

	// Verify structure
	assert.Equal(t, 2, loaded.Version)
	assert.Len(t, loaded.Worktrees, 1)
	assert.Len(t, loaded.Experiments, 1)
	assert.Len(t, loaded.Projects, 1)
	assert.Len(t, loaded.Scratches, 1)

	// Verify worktree data
	wt := loaded.GetWorktree("repo1", "wt1")
	require.NotNil(t, wt)
	assert.Equal(t, "wt1", wt.Name)
	assert.Equal(t, "feature", wt.Label)
	assert.Equal(t, "PROJ-123", wt.Ticket)

	// Verify experiment data
	exp := loaded.GetExperiment("repo-exp1")
	require.NotNil(t, exp)
	assert.Equal(t, "exp1", exp.Name)

	// Verify project data
	proj := loaded.Projects["proj1"]
	require.NotNil(t, proj)
	assert.Equal(t, "proj1", proj.Name)
	assert.Len(t, proj.Repos, 1)

	// Verify scratch data
	scratch := loaded.GetScratch("scratch1")
	require.NotNil(t, scratch)
	assert.Equal(t, "scratch1", scratch.Name)
}

func TestWorkTreePath(t *testing.T) {
	cfg := &Config{BaseDir: "/tmp/pacer"}

	path := WorktreePath(cfg, "my-repo", "test-worktree")

	assert.Equal(t, "/tmp/pacer/repos/my-repo/test-worktree", path)
}

func TestFindExperimentByName(t *testing.T) {
	state := &State{
		Version: 1,
		Experiments: map[string]*Experiment{
			"repo1-exp1": {Name: "exp1", Repo: "/repo1"},
			"repo2-exp2": {Name: "exp2", Repo: "/repo2"},
			"repo1-exp3": {Name: "exp3", Repo: "/repo1"},
		},
	}

	// Find existing experiment
	exp := state.FindExperimentByName("exp2")
	require.NotNil(t, exp)
	assert.Equal(t, "exp2", exp.Name)

	// Find non-existent
	exp = state.FindExperimentByName("nonexistent")
	assert.Nil(t, exp)
}
