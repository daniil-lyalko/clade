package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/pacer/internal/config"
	"github.com/daniil-lyalko/pacer/internal/git"
	"github.com/daniil-lyalko/pacer/internal/ui"
	"github.com/spf13/cobra"
)

var initForceFlag bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Setup a repo for pacer",
	Long: `Initialize a repository for pacer.

This creates:
  - .pacer/hooks.yaml.example (lifecycle hooks template)
  - Appends .pacer/ to .gitignore

For global Claude Code and Cursor hooks, run: pacer setup`,
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

	ui.Header("Initializing pacer in %s", git.GetRepoName(repoRoot))

	// Write hooks.yaml.example template
	pacerDir := filepath.Join(repoRoot, ".pacer")
	if err := os.MkdirAll(pacerDir, 0755); err != nil {
		ui.Warn("Failed to create .pacer/: %v", err)
	} else {
		hooksExamplePath := filepath.Join(pacerDir, "hooks.yaml.example")
		if _, err := os.Stat(hooksExamplePath); os.IsNotExist(err) || initForceFlag {
			ui.Info("Creating .pacer/hooks.yaml.example...")
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

	ui.Success("Pacer initialized!")
	ui.Detail("Run 'pacer setup' to install global hooks (one-time)")

	return nil
}

// dropCommandContent is the shared content for /drop command (same for Claude Code and Cursor)
const dropCommandContent = `Create a timestamped session summary in .pacer/dropbags/:

1. Create directory if needed:
` + "   ```bash" + `
   mkdir -p .pacer/dropbags
` + "   ```" + `

2. Optionally read the most recent DROPBAG for continuity:
` + "   ```bash" + `
   ls -t .pacer/dropbags/DROPBAG-*.md 2>/dev/null | head -1
` + "   ```" + `

3. Write new timestamped file:
` + "   ```bash" + `
   TIMESTAMP=$(date +%Y-%m-%d-%H%M)
   cat > .pacer/dropbags/DROPBAG-$TIMESTAMP.md <<'EOF'
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

// hooksExampleContent is the template for hooks.yaml.example
const hooksExampleContent = `# Pacer Lifecycle Hooks
# Rename to hooks.yaml to activate
# See USER_GUIDE.md for details

hooks:
  # Runs after creating a new worktree
  on_create:
    # - npm install
    # - cp .env.example .env
    # - echo "Created $PACER_NAME"

  # Runs when resuming a worktree
  on_resume:
    # - direnv allow
    # - echo "Resumed at $(date)"

  # Runs before removing a worktree
  on_remove:
    # - echo "Cleaning up $PACER_NAME"

# Available environment variables:
# PACER_TYPE, PACER_NAME, PACER_PATH, PACER_REPO_NAME,
# PACER_REPO_PATH, PACER_BRANCH, PACER_TICKET, PACER_PROJECT_NAME
`

func writeHooksExample(path string) error {
	return os.WriteFile(path, []byte(hooksExampleContent), 0644)
}

func updateGitignore(path string) error {
	linesToAdd := []string{
		".pacer/",
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

	// Add comment header if we're adding pacer entries
	if !strings.Contains(existingContent, "# Pacer") {
		if _, err := f.WriteString("\n# Pacer\n"); err != nil {
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

// InitRepo initializes a repo for pacer (used by other commands).
// Only creates .pacer/ directory and updates .gitignore.
func InitRepo(repoPath string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Only auto-init if configured
	if !cfg.AutoInit {
		return nil
	}

	// Check if already initialized
	pacerDir := filepath.Join(repoPath, ".pacer")
	if _, err := os.Stat(pacerDir); err == nil {
		return nil // Already initialized
	}

	// Create .pacer/ with hooks.yaml.example
	if err := os.MkdirAll(pacerDir, 0755); err != nil {
		return err
	}

	hooksExamplePath := filepath.Join(pacerDir, "hooks.yaml.example")
	if _, err := os.Stat(hooksExamplePath); os.IsNotExist(err) {
		writeHooksExample(hooksExamplePath)
	}

	gitignorePath := filepath.Join(repoPath, ".gitignore")
	return updateGitignore(gitignorePath)
}
