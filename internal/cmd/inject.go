package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/daniil-lyalko/clade/internal/context"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/spf13/cobra"
)

var injectJSONFlag bool

var injectCmd = &cobra.Command{
	Use:    "inject-context",
	Short:  "Output session context (called by hooks)",
	Hidden: true, // Not meant to be called directly by users
	Long: `Outputs session context to stdout for the AI agent to consume.

This command is called automatically by hooks configured by 'clade init':
  - Claude Code: SessionStart hook (plain text output)
  - Cursor: sessionStart hook (JSON output with --json flag)

It gathers:
  - DROPBAG.md contents (session handoff notes)
  - Git status and recent commits
  - Open TODOs in the codebase
  - Ticket information from .clade.json

Use --json for Cursor compatibility (wraps context in JSON with additional_context field).`,
	RunE: runInjectContext,
}

func init() {
	rootCmd.AddCommand(injectCmd)
	injectCmd.Flags().BoolVar(&injectJSONFlag, "json", false, "Output as JSON (for Cursor hooks)")
}

func runInjectContext(cmd *cobra.Command, args []string) error {
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Try to get repo root, fall back to cwd
	dir := cwd
	if git.IsGitRepo(cwd) {
		if root, err := git.GetRepoRoot(cwd); err == nil {
			dir = root
		}
	}

	// Gather context
	ctx, err := context.GatherContext(dir)
	if err != nil {
		return fmt.Errorf("failed to gather context: %w", err)
	}

	// Format context
	output := context.FormatContext(ctx)

	if injectJSONFlag {
		// Cursor format: JSON with additional_context field
		result := map[string]interface{}{
			"additional_context": output,
		}
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(result)
	}

	// Claude Code format: plain text to stdout
	fmt.Print(output)
	return nil
}
