package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var configJSONFlag bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or modify clade configuration",
	Long: `Manage clade configuration settings.

Without arguments, re-runs the setup wizard.

Subcommands:
  show         Display current configuration
  set <k> <v>  Set a configuration value
  reset        Reset to default configuration

Settable keys:
  agent        AI agent to launch (claude, cursor, or empty)
  editor       Editor to open (cursor, code, nvim, or empty)
  base_dir     Directory for worktrees (default: ~/clade)
  auto_init    Auto-run 'clade init' on worktree creation (true/false)
  tmux_split   TMUX pane direction (horizontal/vertical)

Examples:
  clade config                     # Re-run setup wizard
  clade config show                # Display config
  clade config show --json         # JSON output for scripting
  clade config set agent cursor    # Change agent
  clade config set auto_init false # Disable auto-init
  clade config reset               # Reset to defaults`,
	RunE: runConfigWizard,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	RunE:  runConfigShow,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a single configuration value.

Settable keys:
  agent        AI agent to launch (claude, cursor, or empty string "")
  editor       Editor to open (cursor, code, nvim, or empty string "")
  base_dir     Directory for worktrees (default: ~/clade)
  auto_init    Auto-run 'clade init' (true/false)
  tmux_split   TMUX pane direction (horizontal/vertical)

Examples:
  clade config set agent claude
  clade config set editor cursor
  clade config set auto_init false
  clade config set agent ""         # Clear agent setting`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	Long: `Reset configuration to default values.

This will:
  - Clear agent and editor settings
  - Reset base_dir to ~/clade
  - Reset auto_init to true
  - Reset tmux_split to horizontal
  - Preserve registered repos

You will be asked for confirmation before changes are made.`,
	RunE: runConfigReset,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configResetCmd)

	configShowCmd.Flags().BoolVar(&configJSONFlag, "json", false, "Output as JSON")
}

// runConfigWizard re-runs the first-run wizard
func runConfigWizard(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println()
	fmt.Println("  Clade configuration wizard")
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
		// User cancelled
		return nil
	}

	// Track whether we need to ask about editor separately
	askEditor := false

	switch idx {
	case 0: // Claude Code
		cfg.Agent = "claude"
		cfg.AgentFlags = []string{}
		cfg.Editor = ""
		askEditor = true // Claude Code doesn't set editor, so ask
	case 1: // Cursor
		cfg.Agent = ""
		cfg.Editor = "cursor" // Cursor IS the editor, no need to ask
	case 2: // Both
		cfg.Agent = "claude"
		cfg.AgentFlags = []string{}
		cfg.Editor = "cursor" // Both means Cursor is the editor
	case 3: // Neither
		cfg.Agent = ""
		cfg.Editor = ""
		askEditor = true // No AI tool, ask what editor they want
	}

	// Ask about editor if not already set by AI tool choice
	if askEditor {
		fmt.Println()
		editorPrompt := promptui.Select{
			Label: "What editor/IDE do you use (for opening worktrees)",
			Items: []string{
				"Cursor",
				"VS Code",
				"Neovim",
				"Other/None",
			},
		}

		editorIdx, _, err := editorPrompt.Run()
		if err != nil {
			// User cancelled - save what we have
			if saveErr := cfg.Save(); saveErr != nil {
				return fmt.Errorf("failed to save config: %w", saveErr)
			}
			return nil
		}

		switch editorIdx {
		case 0:
			cfg.Editor = "cursor"
		case 1:
			cfg.Editor = "code"
		case 2:
			cfg.Editor = "nvim"
		case 3:
			cfg.Editor = ""
		}
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	ui.Success("Configuration saved!")
	fmt.Println()

	// Show what was set
	if cfg.Agent != "" {
		ui.Detail("Agent: %s", cfg.Agent)
	}
	if cfg.Editor != "" {
		ui.Detail("Editor: %s", cfg.Editor)
	}
	if cfg.Agent == "" && cfg.Editor == "" {
		ui.Detail("No agent or editor configured (worktree management only)")
	}

	fmt.Println()
	ui.Info("Run 'clade config show' to see all settings")
	ui.Info("Run 'clade config set <key> <value>' to change individual settings")

	return nil
}

// ConfigShowOutput is the JSON output structure for config show
type ConfigShowOutput struct {
	Agent        string            `json:"agent"`
	Editor       string            `json:"editor"`
	BaseDir      string            `json:"base_dir"`
	AutoInit     bool              `json:"auto_init"`
	TmuxSplit    string            `json:"tmux_split"`
	Repos        map[string]string `json:"repos"`
	CustomLabels map[string]config.LabelConfig `json:"custom_labels,omitempty"`
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	configPath, _ := config.ConfigPath()

	if configJSONFlag {
		output := ConfigShowOutput{
			Agent:        cfg.Agent,
			Editor:       cfg.Editor,
			BaseDir:      cfg.BaseDir,
			AutoInit:     cfg.AutoInit,
			TmuxSplit:    cfg.TmuxSplitDirection,
			Repos:        cfg.Repos,
			CustomLabels: cfg.CustomLabels,
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	fmt.Printf("\nConfiguration (%s)\n\n", configPath)

	// Core settings
	printConfigKV("agent", cfg.Agent, "(AI agent to launch)")
	printConfigKV("editor", cfg.Editor, "(editor/IDE to open)")
	printConfigKV("base_dir", cfg.BaseDir, "(worktree directory)")
	printConfigKV("auto_init", strconv.FormatBool(cfg.AutoInit), "(auto-run clade init)")
	printConfigKV("tmux_split", cfg.TmuxSplitDirection, "(pane direction)")

	// Repos
	fmt.Println()
	if len(cfg.Repos) > 0 {
		fmt.Printf("  %s: %d registered\n", ui.Dim("repos"), len(cfg.Repos))
		for name, path := range cfg.Repos {
			fmt.Printf("    %s: %s\n", name, path)
		}
	} else {
		fmt.Printf("  %s: none registered\n", ui.Dim("repos"))
	}

	// Custom labels
	if len(cfg.CustomLabels) > 0 {
		fmt.Println()
		fmt.Printf("  %s:\n", ui.Dim("custom_labels"))
		for name, labelCfg := range cfg.CustomLabels {
			fmt.Printf("    %s: prefix=%s, merge=%v\n", name, labelCfg.BranchPrefix, labelCfg.MergeExpected)
		}
	}

	fmt.Println()
	fmt.Printf("Run '%s' to change settings.\n", ui.Cyan("clade config set <key> <value>"))
	fmt.Printf("Run '%s' to re-run setup wizard.\n", ui.Cyan("clade config"))

	return nil
}

func printConfigKV(key, value, hint string) {
	if value == "" {
		value = ui.Dim("(not set)")
	}
	fmt.Printf("  %-12s %s %s\n", key+":", value, ui.Dim(hint))
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Handle known keys
	switch strings.ToLower(key) {
	case "agent":
		cfg.Agent = value
		ui.Success("Set agent = %q", value)

	case "editor":
		cfg.Editor = value
		ui.Success("Set editor = %q", value)

	case "base_dir", "basedir":
		cfg.BaseDir = value
		ui.Success("Set base_dir = %q", value)

	case "auto_init", "autoinit":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for auto_init: must be true or false")
		}
		cfg.AutoInit = boolVal
		ui.Success("Set auto_init = %v", boolVal)

	case "tmux_split", "tmuxsplit":
		if value != "horizontal" && value != "vertical" {
			return fmt.Errorf("invalid value for tmux_split: must be 'horizontal' or 'vertical'")
		}
		cfg.TmuxSplitDirection = value
		ui.Success("Set tmux_split = %q", value)

	default:
		return fmt.Errorf("unknown config key: %q\n\nSettable keys: agent, editor, base_dir, auto_init, tmux_split", key)
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

func runConfigReset(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println()
	ui.Warn("This will reset configuration to defaults:")
	fmt.Println()
	ui.Detail("agent: %s -> \"\"", cfg.Agent)
	ui.Detail("editor: %s -> \"\"", cfg.Editor)
	ui.Detail("base_dir: %s -> ~/clade", cfg.BaseDir)
	ui.Detail("auto_init: %v -> true", cfg.AutoInit)
	ui.Detail("tmux_split: %s -> horizontal", cfg.TmuxSplitDirection)
	fmt.Println()
	ui.Info("Registered repos will be preserved (%d repos)", len(cfg.Repos))
	fmt.Println()

	// Confirmation prompt
	prompt := promptui.Prompt{
		Label:     "Reset configuration",
		IsConfirm: true,
	}

	_, err = prompt.Run()
	if err != nil {
		ui.Info("Reset cancelled")
		return nil
	}

	// Preserve repos and reset everything else
	repos := cfg.Repos
	repoSettings := cfg.RepoSettings
	customLabels := cfg.CustomLabels

	homeDir, _ := os.UserHomeDir()
	cfg.BaseDir = "~/clade"
	cfg.Agent = ""
	cfg.AgentFlags = []string{}
	cfg.Editor = ""
	cfg.AutoInit = true
	cfg.TmuxSplitDirection = "horizontal"
	cfg.LastRepo = ""

	// Restore preserved settings
	cfg.Repos = repos
	cfg.RepoSettings = repoSettings
	cfg.CustomLabels = customLabels

	// Expand base_dir for the actual default
	_ = homeDir // suppress unused warning

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	ui.Success("Configuration reset to defaults")
	ui.Info("Run 'clade config' to set up your preferences")

	return nil
}
