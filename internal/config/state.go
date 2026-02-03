package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Worktree represents a tracked worktree (v2 format)
type Worktree struct {
	Name     string    `json:"name"`
	Label    string    `json:"label"` // feature, bug, spike, chore, hotfix, docs, or custom
	Branch   string    `json:"branch"`
	Path     string    `json:"path,omitempty"` // Actual path if not in ~/clade/repos/ (for adopted worktrees)
	Ticket   string    `json:"ticket,omitempty"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
}

// Experiment represents a tracked experiment (v1 format, deprecated)
// Kept for backward compatibility during migration
type Experiment struct {
	Name     string    `json:"name"`
	Repo     string    `json:"repo"`
	Path     string    `json:"path"`
	Branch   string    `json:"branch"`
	Ticket   string    `json:"ticket,omitempty"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
}

// ProjectRepo represents a repo within a project
type ProjectRepo struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// Project represents a tracked multi-repo project
type Project struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	Branch   string        `json:"branch"`
	Repos    []ProjectRepo `json:"repos"`
	Created  time.Time     `json:"created"`
	LastUsed time.Time     `json:"last_used"`
}

// Scratch represents a no-git scratch folder
type Scratch struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Ticket   string    `json:"ticket,omitempty"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
}

// State holds the runtime state of clade
type State struct {
	Version int `json:"version"`

	// v2 format: nested worktrees by repo name
	// map[repoName]map[worktreeName]*Worktree
	Worktrees map[string]map[string]*Worktree `json:"worktrees,omitempty"`

	// v1 format (deprecated): flat experiments map
	// Kept for backward compatibility during migration
	Experiments map[string]*Experiment `json:"experiments,omitempty"`

	Projects  map[string]*Project `json:"projects"`
	Scratches map[string]*Scratch `json:"scratches,omitempty"`
}

// testStateDir allows tests to override the state directory
// This is only used in tests; production code uses the default path
var testStateDir string

// SetTestStateDir sets a custom state directory for testing
// Pass empty string to reset to default behavior
func SetTestStateDir(dir string) {
	testStateDir = dir
}

// StatePath returns the path to the state file
// State is stored in ~/.config/clade/ alongside other metadata
func StatePath() string {
	if testStateDir != "" {
		return filepath.Join(testStateDir, "state.json")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "clade", "state.json")
}

// LegacyStatePath returns the old state file location (in base_dir)
// Used for auto-migration from v2 to v3
func LegacyStatePath(cfg *Config) string {
	return filepath.Join(cfg.GetBaseDir(), "state.json")
}

// NewState creates a new empty state with default values
func NewState() *State {
	return &State{
		Version:     2,
		Worktrees:   make(map[string]map[string]*Worktree),
		Experiments: make(map[string]*Experiment),
		Projects:    make(map[string]*Project),
		Scratches:   make(map[string]*Scratch),
	}
}

// LoadState reads the state from disk
// Automatically migrates from legacy location if needed
func LoadState(cfg *Config) (*State, error) {
	newPath := StatePath()
	oldPath := LegacyStatePath(cfg)

	// Auto-migration: move from old location if exists and new doesn't
	if _, err := os.Stat(oldPath); err == nil {
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			// Old exists, new doesn't - migrate silently
			if err := os.MkdirAll(filepath.Dir(newPath), 0755); err == nil {
				os.Rename(oldPath, newPath)
			}
		}
	}

	state := &State{
		Version:     2,
		Worktrees:   make(map[string]map[string]*Worktree),
		Experiments: make(map[string]*Experiment),
		Projects:    make(map[string]*Project),
		Scratches:   make(map[string]*Scratch),
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, state); err != nil {
		return nil, err
	}

	// Ensure maps are initialized
	if state.Worktrees == nil {
		state.Worktrees = make(map[string]map[string]*Worktree)
	}
	if state.Experiments == nil {
		state.Experiments = make(map[string]*Experiment)
	}
	if state.Projects == nil {
		state.Projects = make(map[string]*Project)
	}
	if state.Scratches == nil {
		state.Scratches = make(map[string]*Scratch)
	}

	return state, nil
}

// Save writes the state to disk
// Note: cfg parameter kept for backward compatibility but is no longer used
// as state is now saved to ~/.config/clade/state.json
func (s *State) Save(cfg *Config) error {
	statePath := StatePath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	// Security: State files should be owner-only (not world-readable)
	return os.WriteFile(statePath, data, 0600)
}

// AddWorktree adds or updates a worktree in v2 state
func (s *State) AddWorktree(repoName string, wt *Worktree) {
	if s.Worktrees[repoName] == nil {
		s.Worktrees[repoName] = make(map[string]*Worktree)
	}
	s.Worktrees[repoName][wt.Name] = wt
}

// GetWorktree retrieves a worktree by repo name and worktree name
func (s *State) GetWorktree(repoName, name string) *Worktree {
	if s.Worktrees[repoName] == nil {
		return nil
	}
	return s.Worktrees[repoName][name]
}

// RemoveWorktree removes a worktree from v2 state
func (s *State) RemoveWorktree(repoName, name string) {
	if s.Worktrees[repoName] != nil {
		delete(s.Worktrees[repoName], name)
		// Clean up empty repo map
		if len(s.Worktrees[repoName]) == 0 {
			delete(s.Worktrees, repoName)
		}
	}
}

// FindWorktreeByName searches for a worktree by name across all repos
// Returns (repoName, worktree) or ("", nil) if not found
func (s *State) FindWorktreeByName(name string) (string, *Worktree) {
	for repoName, worktrees := range s.Worktrees {
		if wt, ok := worktrees[name]; ok {
			return repoName, wt
		}
	}
	return "", nil
}

// WorktreePath returns the path for a worktree given config and repo name
func WorktreePath(cfg *Config, repoName, name string) string {
	return filepath.Join(cfg.ReposDir(), repoName, name)
}

// GetWorktreePath returns the actual path for a worktree, using custom Path if set
func GetWorktreePath(cfg *Config, repoName string, wt *Worktree) string {
	if wt.Path != "" {
		return wt.Path
	}
	return WorktreePath(cfg, repoName, wt.Name)
}

// AddExperiment adds or updates an experiment in state (v1 format, deprecated)
func (s *State) AddExperiment(exp *Experiment) {
	key := ExperimentKey(exp.Repo, exp.Name)
	s.Experiments[key] = exp
}

// GetExperiment retrieves an experiment by key (v1 format)
func (s *State) GetExperiment(key string) *Experiment {
	return s.Experiments[key]
}

// RemoveExperiment removes an experiment from state (v1 format)
func (s *State) RemoveExperiment(key string) {
	delete(s.Experiments, key)
}

// ExperimentKey generates a unique key for an experiment (v1 format)
func ExperimentKey(repo, name string) string {
	repoName := filepath.Base(repo)
	return repoName + "-" + name
}

// FindExperimentByName searches for an experiment by name in v1 format
// Returns the experiment or nil if not found
func (s *State) FindExperimentByName(name string) *Experiment {
	for _, exp := range s.Experiments {
		if exp.Name == name {
			return exp
		}
	}
	return nil
}

// AddScratch adds or updates a scratch in state
func (s *State) AddScratch(scratch *Scratch) {
	s.Scratches[scratch.Name] = scratch
}

// GetScratch retrieves a scratch by name
func (s *State) GetScratch(name string) *Scratch {
	return s.Scratches[name]
}

// RemoveScratch removes a scratch from state
func (s *State) RemoveScratch(name string) {
	delete(s.Scratches, name)
}
