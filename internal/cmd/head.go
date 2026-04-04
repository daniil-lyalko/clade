package cmd

import "github.com/spf13/cobra"

var headCmd = &cobra.Command{
	Use:   "head",
	Short: "Manage the persistent orchestrator session",
	Long: `Manage the persistent Head session - a long-running Claude Code instance
that acts as an orchestrator for your machine's coding sessions.

Commands:
  init     Create ~/.clade/head/ with default CLAUDE.md
  start    Launch claude --remote-control in a tmux session
  stop     Gracefully stop the head session
  status   Show running state and info
  attach   Attach to the tmux session`,
}

func init() {
	headCmd.AddCommand(headInitCmd)
	headCmd.AddCommand(headStartCmd)
	headCmd.AddCommand(headStopCmd)
	headCmd.AddCommand(headStatusCmd)
	headCmd.AddCommand(headAttachCmd)
	rootCmd.AddCommand(headCmd)
}
