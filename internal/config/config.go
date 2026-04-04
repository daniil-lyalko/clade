package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/manifoldco/promptui"
)

// RepoSettings holds per-repo configuration
type RepoSettings struct {
	CopyFiles []string `json:"copy_files,omitempty"`
}

// LabelConfig defines a custom label configuration
type LabelConfig struct {
	BranchPrefix  string `json:"branch_prefix"`
	MergeExpected bool   `json:"merge_expected"`
}

// Config holds the user configuration for clade
type Config struct {
	BaseDir            string                  `json:"base_dir"`
	Agent              string                  `json:"agent"`
	AgentFlags         []string                `json:"agent_flags"`
	Editor             string                  `json:"editor,omitempty"`
	AutoInit           bool                    `json:"auto_init"`
	Repos              map[string]string       `json:"repos"`
	RepoSettings       map[string]RepoSettings `json:"repo_settings,omitempty"`
	LastRepo           string                  `json:"last_repo"`
	TmuxSplitDirection string                  `json:"tmux_split_direction,omitempty"`
	CustomLabels       map[string]LabelConfig  `json:"custom_labels,omitempty"`
	CopyFiles          []string                `json:"copy_files,omitempty"` // Additional files to copy to worktrees (beyond defaults)
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		BaseDir:            filepath.Join(homeDir, "clade"),
		Agent:              "",  // Set by first-run wizard
		AgentFlags:         []string{},
		Editor:             "",  // Set by first-run wizard
		AutoInit:           true,
		Repos:              make(map[string]string),
		RepoSettings:       make(map[string]RepoSettings),
		LastRepo:           "",
		TmuxSplitDirection: "horizontal",
		CustomLabels:       make(map[string]LabelConfig),
	}
}

// ConfigPath returns the path to the config file
// Always uses ~/.config/clade/ for consistency across platforms
func ConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", "clade", "config.json"), nil
}

// legacyConfigPath returns the old platform-specific config path (for migration)
// macOS: ~/Library/Application Support/clade/config.json
// Linux: ~/.config/clade/config.json (same as new path)
func legacyConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "clade", "config.json"), nil
}

// Load reads the config from disk, creating default if not exists
func Load() (*Config, error) {
	configPath, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Check for legacy config path (platform-specific) and migrate
			if migrated, migrateErr := migrateFromLegacyPath(configPath); migrateErr == nil && migrated {
				// Successfully migrated - reload from new path
				data, err = os.ReadFile(configPath)
				if err != nil {
					return nil, err
				}
			} else {
				// No legacy config or migration failed - run first-run wizard
				if err := runFirstRunWizard(cfg); err != nil {
					return nil, err
				}
				if err := cfg.Save(); err != nil {
					return nil, err
				}
				// After first-run wizard, check if user is in a git repo and offer to register it
				if err := offerRepoRegistration(cfg); err != nil {
					// Non-fatal - just print warning
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				}
				return cfg, nil
			}
		} else {
			return nil, err
		}
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Ensure maps are initialized
	if cfg.Repos == nil {
		cfg.Repos = make(map[string]string)
	}
	if cfg.RepoSettings == nil {
		cfg.RepoSettings = make(map[string]RepoSettings)
	}
	if cfg.CustomLabels == nil {
		cfg.CustomLabels = make(map[string]LabelConfig)
	}

	return cfg, nil
}

// migrateFromLegacyPath checks for config at legacy platform-specific path and migrates to ~/.config/clade/
// Returns (migrated bool, error)
func migrateFromLegacyPath(newPath string) (bool, error) {
	legacyPath, err := legacyConfigPath()
	if err != nil {
		return false, err
	}

	// If legacy path is the same as new path (Linux), no migration needed
	if legacyPath == newPath {
		return false, nil
	}

	// Check if legacy config exists
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // No legacy config
		}
		return false, err
	}

	// Create new directory and copy config
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return false, err
	}

	// Security: Config files should be owner-only (not world-readable)
	if err := os.WriteFile(newPath, data, 0600); err != nil {
		return false, err
	}

	// Migration successful - try to remove old file (don't fail if this doesn't work)
	os.Remove(legacyPath)
	// Try to remove old directory if empty
	os.Remove(filepath.Dir(legacyPath))

	return true, nil
}

// runFirstRunWizard prompts the user for initial configuration
func runFirstRunWizard(cfg *Config) error {
	fmt.Println()
	fmt.Println("  Welcome to clade! Let's set up your preferences.")
	fmt.Println()

	// Ask about AI coding tool
	toolPrompt := promptui.Select{
		Label: "What AI coding tool do you use",
		Items: []string{
			"Claude Code (terminal)",
			"Cursor (IDE)",
			"Both Claude Code and Cursor",
			"Neither (just worktree management)",
		},
	}

	idx, _, err := toolPrompt.Run()
	if err != nil {
		// User cancelled - use defaults
		return nil
	}

	switch idx {
	case 0: // Claude Code
		cfg.Agent = "claude"
		// Security: Don't add --dangerously-skip-permissions by default
		// Users can add it manually in config.json if needed
		cfg.AgentFlags = []string{}
		cfg.Editor = ""
	case 1: // Cursor
		cfg.Agent = ""
		cfg.Editor = "cursor"
	case 2: // Both
		cfg.Agent = "claude"
		// Security: Don't add --dangerously-skip-permissions by default
		// Users can add it manually in config.json if needed
		cfg.AgentFlags = []string{}
		cfg.Editor = "cursor"
	case 3: // Neither
		cfg.Agent = ""
		cfg.Editor = ""
	}

	fmt.Println()
	fmt.Println("  Configuration saved to ~/.config/clade/config.json")
	fmt.Println("  You can edit it anytime to change these settings.")
	fmt.Println()

	return nil
}

// Save writes the config to disk
func (c *Config) Save() error {
	configPath, err := ConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	// Security: Config files should be owner-only (not world-readable)
	return os.WriteFile(configPath, data, 0600)
}

// ExpandPath expands ~ to home directory
func ExpandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, path[1:])
	}
	return path
}

// GetBaseDir returns the expanded base directory
func (c *Config) GetBaseDir() string {
	return ExpandPath(c.BaseDir)
}

// ReposDir returns the path to repos directory (v0.3+ repo-centric structure)
func (c *Config) ReposDir() string {
	return filepath.Join(c.GetBaseDir(), "repos")
}

// ExperimentsDir returns the path to experiments directory (legacy v0.2 structure)
// Deprecated: Use ReposDir() for new worktrees
func (c *Config) ExperimentsDir() string {
	return filepath.Join(c.GetBaseDir(), "experiments")
}

// ProjectsDir returns the path to projects directory
func (c *Config) ProjectsDir() string {
	return filepath.Join(c.GetBaseDir(), "projects")
}

// ScratchDir returns the path to scratch directory
func (c *Config) ScratchDir() string {
	return filepath.Join(c.GetBaseDir(), "scratch")
}

// ArchiveDir returns the path to the session archive directory
// Archives are stored in ~/.config/clade/archive/ to persist across base_dir changes
func ArchiveDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "clade", "archive")
}

// DotCladeDir returns the path to ~/.clade/ — the unified Clade data directory.
// This is the new canonical location for all Clade state (v0.8+).
func DotCladeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".clade")
	}
	return filepath.Join(homeDir, ".clade")
}

// SessionsDir returns the path to ~/.clade/sessions/
func SessionsDir() string {
	return filepath.Join(DotCladeDir(), "sessions")
}

// InboxDir returns the path to ~/.clade/inbox/
func InboxDir() string {
	return filepath.Join(DotCladeDir(), "inbox")
}

// GetRepoCopyFiles returns the copy_files setting for a repo
func (c *Config) GetRepoCopyFiles(repoPath string) []string {
	if settings, ok := c.RepoSettings[repoPath]; ok {
		return settings.CopyFiles
	}
	return nil
}

// SetRepoCopyFiles saves the copy_files setting for a repo
func (c *Config) SetRepoCopyFiles(repoPath string, files []string) {
	if c.RepoSettings == nil {
		c.RepoSettings = make(map[string]RepoSettings)
	}
	c.RepoSettings[repoPath] = RepoSettings{CopyFiles: files}
}

// BuiltInLabels returns the built-in label configurations
func BuiltInLabels() map[string]LabelConfig {
	return map[string]LabelConfig{
		"feature": {BranchPrefix: "feat", MergeExpected: true},
		"bug":     {BranchPrefix: "fix", MergeExpected: true},
		"spike":   {BranchPrefix: "spike", MergeExpected: false},
		"chore":   {BranchPrefix: "chore", MergeExpected: true},
		"hotfix":  {BranchPrefix: "hotfix", MergeExpected: true},
		"docs":    {BranchPrefix: "docs", MergeExpected: true},
	}
}

// GetLabelConfig returns the configuration for a label (built-in or custom)
func (c *Config) GetLabelConfig(label string) (LabelConfig, bool) {
	// Check built-in labels first
	if cfg, ok := BuiltInLabels()[label]; ok {
		return cfg, true
	}
	// Check custom labels
	if cfg, ok := c.CustomLabels[label]; ok {
		return cfg, true
	}
	return LabelConfig{}, false
}

// offerRepoRegistration detects if the user is in a git repo and offers to register it.
// Called after the first-run wizard completes.
func offerRepoRegistration(cfg *Config) error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil // Can't get cwd, skip silently
	}

	if !git.IsGitRepo(cwd) {
		return nil // Not in a git repo, nothing to do
	}

	repoRoot, err := git.GetRepoRoot(cwd)
	if err != nil {
		return nil // Can't get repo root, skip
	}

	repoName := git.GetRepoName(repoRoot)

	// Check if already registered
	for _, path := range cfg.Repos {
		if ExpandPath(path) == repoRoot {
			return nil // Already registered
		}
	}

	// Offer to register
	fmt.Println()
	fmt.Printf("  Detected git repository: %s\n", repoName)
	fmt.Printf("  Path: %s\n", repoRoot)
	fmt.Println()

	prompt := promptui.Prompt{
		Label:     "Register this repository for quick access",
		IsConfirm: true,
		Default:   "y",
	}

	if _, err := prompt.Run(); err != nil {
		// User declined
		fmt.Println("  Skipped. You can register it later with: clade repo add <path>")
		return nil
	}

	// Register the repo
	cfg.Repos[repoName] = repoRoot
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("  Registered '%s'\n", repoName)
	return nil
}
