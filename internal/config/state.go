package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Experiment represents a tracked experiment
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
	Version     int                    `json:"version"`
	Experiments map[string]*Experiment `json:"experiments"`
	Projects    map[string]*Project    `json:"projects"`
	Scratches   map[string]*Scratch    `json:"scratches,omitempty"`
}

// StatePath returns the path to the state file
func StatePath(cfg *Config) string {
	return filepath.Join(cfg.GetBaseDir(), "state.json")
}

// LoadState reads the state from disk
func LoadState(cfg *Config) (*State, error) {
	statePath := StatePath(cfg)

	state := &State{
		Version:     1,
		Experiments: make(map[string]*Experiment),
		Projects:    make(map[string]*Project),
		Scratches:   make(map[string]*Scratch),
	}

	data, err := os.ReadFile(statePath)
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
func (s *State) Save(cfg *Config) error {
	statePath := StatePath(cfg)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, data, 0644)
}

// AddExperiment adds or updates an experiment in state
func (s *State) AddExperiment(exp *Experiment) {
	key := ExperimentKey(exp.Repo, exp.Name)
	s.Experiments[key] = exp
}

// GetExperiment retrieves an experiment by key
func (s *State) GetExperiment(key string) *Experiment {
	return s.Experiments[key]
}

// RemoveExperiment removes an experiment from state
func (s *State) RemoveExperiment(key string) {
	delete(s.Experiments, key)
}

// ExperimentKey generates a unique key for an experiment
func ExperimentKey(repo, name string) string {
	repoName := filepath.Base(repo)
	return repoName + "-" + name
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
