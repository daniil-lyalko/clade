package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var initForceFlag bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Setup a repo for clade with hooks",
	Long: `Initialize a repository for clade by generating .claude/ configuration.

This creates:
  - .claude/settings.json with SessionStart hook
  - .claude/commands/drop.md for the /drop command
  - Appends DROPBAG.md and .clade.json to .gitignore

Run this in any git repository to enable context injection.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&initForceFlag, "force", "f", false, "Overwrite existing configuration")
}

func runInit(cmd *cobra.Command, args []string) error {
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Verify we're in a git repo
	if !git.IsGitRepo(cwd) {
		return fmt.Errorf("not a git repository")
	}

	repoRoot, err := git.GetRepoRoot(cwd)
	if err != nil {
		return err
	}

	claudeDir := filepath.Join(repoRoot, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")

	// Check if already initialized
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if _, err := os.Stat(settingsPath); err == nil && !initForceFlag {
		ui.Warn(".claude/settings.json already exists")
		ui.Detail("Use --force to overwrite")
		return nil
	}

	ui.Header("Initializing clade in %s", git.GetRepoName(repoRoot))

	// Create directories
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude/commands: %w", err)
	}

	// Write settings.json
	ui.Info("Creating .claude/settings.json...")
	if err := writeSettingsJSON(settingsPath); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	// Write drop.md command
	dropPath := filepath.Join(commandsDir, "drop.md")
	ui.Info("Creating .claude/commands/drop.md...")
	if err := writeDropCommand(dropPath); err != nil {
		return fmt.Errorf("failed to write drop.md: %w", err)
	}

	// Update .gitignore
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	ui.Info("Updating .gitignore...")
	if err := updateGitignore(gitignorePath); err != nil {
		ui.Warn("Failed to update .gitignore: %v", err)
	}

	ui.Success("Clade initialized!")
	ui.Detail("SessionStart hook will call: clade inject-context")
	ui.Detail("Use /drop to save session context before stopping")

	return nil
}

func writeSettingsJSON(path string) error {
	content := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "clade inject-context"
          }
        ]
      }
    ]
  }
}
`
	return os.WriteFile(path, []byte(content), 0644)
}

func writeDropCommand(path string) error {
	content := `Write a DROPBAG.md file in the repo root with the following sections:

## Summary
What we accomplished this session. Be specific about changes made.

## Current State
What's working, what's broken, what's partially implemented.

## Next Steps
Exact actions to continue (be specific - file names, function names, etc.).

## Key Files
Files to look at first when resuming. Include line numbers if relevant.

## Open Questions
Anything unresolved or decisions that need to be made.

---

Save the file to DROPBAG.md in the repository root, then confirm it's written.
`
	return os.WriteFile(path, []byte(content), 0644)
}

func updateGitignore(path string) error {
	linesToAdd := []string{
		"DROPBAG.md",
		".clade.json",
	}

	// Read existing content
	var existingContent string
	if data, err := os.ReadFile(path); err == nil {
		existingContent = string(data)
	}

	// Check which lines need to be added
	var toAdd []string
	for _, line := range linesToAdd {
		if !strings.Contains(existingContent, line) {
			toAdd = append(toAdd, line)
		}
	}

	if len(toAdd) == 0 {
		return nil // Nothing to add
	}

	// Append to file
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newline if file doesn't end with one
	if len(existingContent) > 0 && !strings.HasSuffix(existingContent, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	// Add comment header if we're adding clade entries
	if !strings.Contains(existingContent, "# Clade") {
		if _, err := f.WriteString("\n# Clade\n"); err != nil {
			return err
		}
	}

	// Add the lines
	for _, line := range toAdd {
		if _, err := f.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	return nil
}

// InitRepo initializes a repo for clade (used by other commands like exp)
func InitRepo(repoPath string) error {
	claudeDir := filepath.Join(repoPath, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Skip if already initialized
	if _, err := os.Stat(settingsPath); err == nil {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Only auto-init if configured
	if !cfg.AutoInit {
		return nil
	}

	commandsDir := filepath.Join(claudeDir, "commands")
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	if err := writeSettingsJSON(settingsPath); err != nil {
		return err
	}

	dropPath := filepath.Join(commandsDir, "drop.md")
	if err := writeDropCommand(dropPath); err != nil {
		return err
	}

	gitignorePath := filepath.Join(repoPath, ".gitignore")
	return updateGitignore(gitignorePath)
}
