package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Event represents a hook event type
type Event string

const (
	OnCreate Event = "on_create"
	OnResume Event = "on_resume"
	OnRemove Event = "on_remove"
)

// HooksConfig represents the structure of hooks.yaml
type HooksConfig struct {
	Hooks map[Event][]string `yaml:"hooks"`
}

// Env holds environment variables available to hooks
type Env struct {
	Type        string // spike, feature, bug, chore, hotfix, docs
	Name        string // worktree name
	Path        string // worktree path
	RepoName    string // repository name
	RepoPath    string // source repository path
	Branch      string // branch name
	Ticket      string // ticket ID (if detected)
	ProjectName string // project name (for multi-repo projects)
}

// toEnvVars converts Env to environment variable strings
func (e *Env) toEnvVars() []string {
	vars := []string{
		fmt.Sprintf("CLADE_TYPE=%s", e.Type),
		fmt.Sprintf("CLADE_NAME=%s", e.Name),
		fmt.Sprintf("CLADE_PATH=%s", e.Path),
		fmt.Sprintf("CLADE_REPO_NAME=%s", e.RepoName),
		fmt.Sprintf("CLADE_REPO_PATH=%s", e.RepoPath),
		fmt.Sprintf("CLADE_BRANCH=%s", e.Branch),
	}
	if e.Ticket != "" {
		vars = append(vars, fmt.Sprintf("CLADE_TICKET=%s", e.Ticket))
	}
	if e.ProjectName != "" {
		vars = append(vars, fmt.Sprintf("CLADE_PROJECT_NAME=%s", e.ProjectName))
	}
	return vars
}

// globalHooksPath returns the path to global hooks config
// Always uses ~/.config/clade/ for consistency across platforms
func globalHooksPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", "clade", "hooks.yaml"), nil
}

// repoHooksPath returns the path to repo-specific hooks config
func repoHooksPath(repoPath string) string {
	return filepath.Join(repoPath, ".clade", "hooks.yaml")
}

// loadHooksConfig loads hooks from a YAML file
func loadHooksConfig(path string) (*HooksConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &HooksConfig{Hooks: make(map[Event][]string)}, nil
		}
		return nil, err
	}

	var config HooksConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if config.Hooks == nil {
		config.Hooks = make(map[Event][]string)
	}

	return &config, nil
}

// Result represents the result of running a hook
type Result struct {
	Command  string
	Output   string
	Error    error
	Duration time.Duration
}

// RunHooks runs hooks for a given event
// Returns results for each hook (for logging/display purposes)
// Hooks that fail do NOT stop subsequent hooks from running
func RunHooks(event Event, env *Env) []Result {
	var results []Result

	// Load global hooks
	globalPath, err := globalHooksPath()
	if err == nil {
		globalConfig, err := loadHooksConfig(globalPath)
		if err == nil {
			for _, cmd := range globalConfig.Hooks[event] {
				result := runSingleHook(cmd, env)
				results = append(results, result)
			}
		}
	}

	// Load repo-specific hooks (run after global hooks)
	if env.RepoPath != "" {
		repoConfig, err := loadHooksConfig(repoHooksPath(env.RepoPath))
		if err == nil {
			for _, cmd := range repoConfig.Hooks[event] {
				// Commands starting with "!" skip global hooks (handled above)
				// TrimPrefix handles the check internally, no need for if statement
				cmd = strings.TrimPrefix(cmd, "!")
				result := runSingleHook(cmd, env)
				results = append(results, result)
			}
		}
	}

	return results
}

// runSingleHook executes a single hook command
func runSingleHook(command string, env *Env) Result {
	start := time.Now()
	result := Result{Command: command}

	// Create context with timeout (30 seconds default)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run via shell to support pipes, redirects, etc.
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = env.Path

	// Set up environment
	cmd.Env = append(os.Environ(), env.toEnvVars()...)

	// Capture output
	output, err := cmd.CombinedOutput()
	result.Output = string(output)
	result.Error = err
	result.Duration = time.Since(start)

	return result
}

// HasHooks checks if there are any hooks configured for the given event
func HasHooks(event Event, repoPath string) bool {
	// Check global hooks
	globalPath, err := globalHooksPath()
	if err == nil {
		globalConfig, err := loadHooksConfig(globalPath)
		if err == nil && len(globalConfig.Hooks[event]) > 0 {
			return true
		}
	}

	// Check repo-specific hooks
	if repoPath != "" {
		repoConfig, err := loadHooksConfig(repoHooksPath(repoPath))
		if err == nil && len(repoConfig.Hooks[event]) > 0 {
			return true
		}
	}

	return false
}
