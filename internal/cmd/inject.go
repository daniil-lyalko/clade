package cmd

import (
	"fmt"
	"os"

	"github.com/daniil-lyalko/clade/internal/context"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/spf13/cobra"
)

var injectCmd = &cobra.Command{
	Use:    "inject-context",
	Short:  "Output session context (called by SessionStart hook)",
	Hidden: true, // Not meant to be called directly by users
	Long: `Outputs session context to stdout for Claude to consume.

This command is called automatically by the SessionStart hook configured
by 'clade init'. It gathers:
  - DROPBAG.md contents (session handoff notes)
  - Git status and recent commits
  - Open TODOs in the codebase
  - Ticket information from .clade.json

The output is formatted as markdown for Claude to read.`,
	RunE: runInjectContext,
}

func init() {
	rootCmd.AddCommand(injectCmd)
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

	// Format and output
	output := context.FormatContext(ctx)
	fmt.Print(output)

	return nil
}
