package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var headInitBrainFlag string

var headInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create ~/.clade/head/ with default CLAUDE.md",
	RunE:  runHeadInit,
}

func init() {
	headInitCmd.Flags().StringVar(&headInitBrainFlag, "brain", "", "Path to knowledge base directory (creates symlink)")
}

const defaultHeadClaudeMD = `# Head Session

You are the orchestrator for this machine's Claude Code sessions.

## Capabilities
- Manage sessions via ` + "`clade start`" + `, ` + "`clade sessions`" + `, ` + "`clade stop`" + `
- Read cross-session updates from ~/.clade/inbox/
- Spawn coding sessions for implementation tasks
- Access your brain/knowledge base (if configured in ./brain/)

## Customization
- Edit this file to define your assistant's personality and behavior
- Add skills to .claude/skills/ for persistent capabilities
- Create a brain/ directory or symlink for knowledge base
- Configure Channels for messaging (Telegram, Discord, etc.)

## Session Awareness
On each context refresh, check ~/.clade/inbox/ for updates from other sessions.
When spawning sessions, use ` + "`clade start <repo> --ticket <id>`" + ` so they're tracked.
When sessions complete, their summaries appear in the inbox automatically.
`

func runHeadInit(cmd *cobra.Command, args []string) error {
	headDir := headDirectory()
	return doHeadInit(headDir, headInitBrainFlag)
}

// doHeadInit creates the head directory structure. Extracted for testability.
func doHeadInit(headDir, brainPath string) error {
	// Check if already initialized
	if _, err := os.Stat(headDir); err == nil {
		ui.Info("Head already initialized at %s", headDir)
		return nil
	}

	// Create directory structure
	skillsDir := filepath.Join(headDir, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create head directory: %w", err)
	}

	// Write default CLAUDE.md
	claudeMDPath := filepath.Join(headDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte(defaultHeadClaudeMD), 0644); err != nil {
		return fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}

	// Handle --brain flag
	if brainPath != "" {
		absPath, err := filepath.Abs(brainPath)
		if err != nil {
			return fmt.Errorf("failed to resolve brain path: %w", err)
		}
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return fmt.Errorf("brain path does not exist: %s", absPath)
		}
		brainLink := filepath.Join(headDir, "brain")
		if err := os.Symlink(absPath, brainLink); err != nil {
			return fmt.Errorf("failed to create brain symlink: %w", err)
		}
		ui.Success("Linked brain: %s", absPath)
	}

	ui.Success("Head initialized at %s", headDir)
	ui.Detail("Edit %s to customize your orchestrator", filepath.Join(headDir, "CLAUDE.md"))
	ui.Detail("Run `clade head start` to launch the session")

	return nil
}

// headDirectory returns the path to ~/.clade/head/
func headDirectory() string {
	return filepath.Join(cladeBaseDir(), "head")
}
