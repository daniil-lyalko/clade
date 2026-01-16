package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// RepoSettings holds per-repo configuration
type RepoSettings struct {
	CopyFiles []string `json:"copy_files,omitempty"`
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
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		BaseDir:            filepath.Join(homeDir, "clade"),
		Agent:              "claude",
		AgentFlags:         []string{},
		Editor:             "",
		AutoInit:           true,
		Repos:              make(map[string]string),
		RepoSettings:       make(map[string]RepoSettings),
		LastRepo:           "",
		TmuxSplitDirection: "horizontal",
	}
}

// ConfigPath returns the path to the config file
func ConfigPath() (string, error) {
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
			// Config doesn't exist, save default and return
			if err := cfg.Save(); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, err
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

	return cfg, nil
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

	return os.WriteFile(configPath, data, 0644)
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

// ExperimentsDir returns the path to experiments directory
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
