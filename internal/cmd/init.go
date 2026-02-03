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
  - .cursor/hooks.json for Cursor IDE
  - .cursor/commands/drop.md for the /drop command in Cursor
  - Appends .clade/ to .gitignore

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

	// Write Cursor hooks.json and commands/drop.md
	cursorDir := filepath.Join(repoRoot, ".cursor")
	cursorCommandsDir := filepath.Join(cursorDir, "commands")
	if err := os.MkdirAll(cursorCommandsDir, 0755); err != nil {
		ui.Warn("Failed to create .cursor/commands/: %v", err)
	} else {
		cursorHooksPath := filepath.Join(cursorDir, "hooks.json")
		// Only write if it doesn't exist or force flag is set
		if _, err := os.Stat(cursorHooksPath); os.IsNotExist(err) || initForceFlag {
			ui.Info("Creating .cursor/hooks.json...")
			if err := writeCursorHooksJSON(cursorHooksPath); err != nil {
				ui.Warn("Failed to write .cursor/hooks.json: %v", err)
			}
		}
		// Write Cursor drop command
		cursorDropPath := filepath.Join(cursorCommandsDir, "drop.md")
		if _, err := os.Stat(cursorDropPath); os.IsNotExist(err) || initForceFlag {
			ui.Info("Creating .cursor/commands/drop.md...")
			if err := writeCursorDropCommand(cursorDropPath); err != nil {
				ui.Warn("Failed to write .cursor/commands/drop.md: %v", err)
			}
		}
	}

	// Write hooks.yaml.example template
	cladeDir := filepath.Join(repoRoot, ".clade")
	if err := os.MkdirAll(cladeDir, 0755); err != nil {
		ui.Warn("Failed to create .clade/: %v", err)
	} else {
		hooksExamplePath := filepath.Join(cladeDir, "hooks.yaml.example")
		if _, err := os.Stat(hooksExamplePath); os.IsNotExist(err) || initForceFlag {
			ui.Info("Creating .clade/hooks.yaml.example...")
			if err := writeHooksExample(hooksExamplePath); err != nil {
				ui.Warn("Failed to write hooks.yaml.example: %v", err)
			}
		}
	}

	// Update .gitignore
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	ui.Info("Updating .gitignore...")
	if err := updateGitignore(gitignorePath); err != nil {
		ui.Warn("Failed to update .gitignore: %v", err)
	}

	ui.Success("Clade initialized!")
	ui.Detail("Claude Code: SessionStart hook calls clade inject-context")
	ui.Detail("Cursor: sessionStart hook calls clade inject-context (format auto-detected)")
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

func writeCursorHooksJSON(path string) error {
	content := `{
  "version": 1,
  "hooks": {
    "sessionStart": [
      {
        "command": "clade inject-context"
      }
    ]
  }
}
`
	return os.WriteFile(path, []byte(content), 0644)
}

// dropCommandContent is the shared content for /drop command (same for Claude Code and Cursor)
const dropCommandContent = `Create a timestamped session summary in .clade/dropbags/:

1. Create directory if needed:
` + "   ```bash" + `
   mkdir -p .clade/dropbags
` + "   ```" + `

2. Optionally read the most recent DROPBAG for continuity:
` + "   ```bash" + `
   ls -t .clade/dropbags/DROPBAG-*.md 2>/dev/null | head -1
` + "   ```" + `

3. Write new timestamped file:
` + "   ```bash" + `
   TIMESTAMP=$(date +%Y-%m-%d-%H%M)
   cat > .clade/dropbags/DROPBAG-$TIMESTAMP.md <<'EOF'
   [your content here]
   EOF
` + "   ```" + `

The new file should contain:

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

After saving, confirm the timestamped file was created successfully.
`

func writeDropCommand(path string) error {
	return os.WriteFile(path, []byte(dropCommandContent), 0644)
}

func writeCursorDropCommand(path string) error {
	// Cursor uses the same markdown format as Claude Code for custom commands
	return os.WriteFile(path, []byte(dropCommandContent), 0644)
}

// hooksExampleContent is the template for hooks.yaml.example
const hooksExampleContent = `# Clade Lifecycle Hooks
# Rename to hooks.yaml to activate
# See USER_GUIDE.md for details

hooks:
  # Runs after creating a new worktree
  on_create:
    # - npm install
    # - cp .env.example .env
    # - echo "Created $CLADE_NAME"

  # Runs when resuming a worktree
  on_resume:
    # - direnv allow
    # - echo "Resumed at $(date)"

  # Runs before removing a worktree
  on_remove:
    # - echo "Cleaning up $CLADE_NAME"

# Available environment variables:
# CLADE_TYPE, CLADE_NAME, CLADE_PATH, CLADE_REPO_NAME,
# CLADE_REPO_PATH, CLADE_BRANCH, CLADE_TICKET, CLADE_PROJECT_NAME
`

func writeHooksExample(path string) error {
	return os.WriteFile(path, []byte(hooksExampleContent), 0644)
}

func updateGitignore(path string) error {
	linesToAdd := []string{
		".clade/",
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

// InitRepo initializes a repo for clade (used by other commands)
func InitRepo(repoPath string) error {
	claudeDir := filepath.Join(repoPath, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Skip if already initialized (check Claude config as primary marker)
	if _, err := os.Stat(settingsPath); err == nil {
		// Claude config exists - still ensure Cursor config
		cursorDir := filepath.Join(repoPath, ".cursor")
		cursorHooksPath := filepath.Join(cursorDir, "hooks.json")
		if _, err := os.Stat(cursorHooksPath); os.IsNotExist(err) {
			os.MkdirAll(cursorDir, 0755)
			writeCursorHooksJSON(cursorHooksPath)
		}
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

	// Claude Code config
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

	// Cursor config
	cursorDir := filepath.Join(repoPath, ".cursor")
	cursorCommandsDir := filepath.Join(cursorDir, "commands")
	if err := os.MkdirAll(cursorCommandsDir, 0755); err != nil {
		return err
	}
	cursorHooksPath := filepath.Join(cursorDir, "hooks.json")
	if err := writeCursorHooksJSON(cursorHooksPath); err != nil {
		return err
	}
	cursorDropPath := filepath.Join(cursorCommandsDir, "drop.md")
	if err := writeCursorDropCommand(cursorDropPath); err != nil {
		return err
	}

	gitignorePath := filepath.Join(repoPath, ".gitignore")
	return updateGitignore(gitignorePath)
}

// EnsureAgentConfig ensures required .claude and .cursor files exist.
// This is called AFTER copying config dirs from source to handle partial configs.
func EnsureAgentConfig(repoPath string) error {
	// Ensure .claude/ config
	claudeDir := filepath.Join(repoPath, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")

	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		if err := writeSettingsJSON(settingsPath); err != nil {
			return err
		}
	}

	dropPath := filepath.Join(commandsDir, "drop.md")
	if _, err := os.Stat(dropPath); os.IsNotExist(err) {
		if err := writeDropCommand(dropPath); err != nil {
			return err
		}
	}

	// Ensure .cursor/ config
	cursorDir := filepath.Join(repoPath, ".cursor")
	cursorCommandsDir := filepath.Join(cursorDir, "commands")
	if err := os.MkdirAll(cursorCommandsDir, 0755); err != nil {
		return err
	}

	cursorHooksPath := filepath.Join(cursorDir, "hooks.json")
	if _, err := os.Stat(cursorHooksPath); os.IsNotExist(err) {
		if err := writeCursorHooksJSON(cursorHooksPath); err != nil {
			return err
		}
	}

	cursorDropPath := filepath.Join(cursorCommandsDir, "drop.md")
	if _, err := os.Stat(cursorDropPath); os.IsNotExist(err) {
		if err := writeCursorDropCommand(cursorDropPath); err != nil {
			return err
		}
	}

	// Ensure gitignore entries
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	return updateGitignore(gitignorePath)
}
