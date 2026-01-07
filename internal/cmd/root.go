package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "clade",
	Short: "Claude Code Workflow CLI",
	Long: `Clade manages git worktrees and context for AI coding sessions.

Named after biological clades (branching groups sharing common ancestry) -
perfect metaphor for worktree branches.

Quick start:
  clade exp try-redis       # Create isolated experiment
  clade list                # See what's active
  clade resume try-redis    # Get back to work
  clade cleanup try-redis   # Clean up when done`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags can be added here
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
}
