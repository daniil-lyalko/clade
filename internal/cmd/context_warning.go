package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var contextWarningCmd = &cobra.Command{
	Use:    "context-warning",
	Short:  "Print context compaction warning (called by PreCompact hook)",
	Hidden: true,
	Long: `Prints a calm notification when context is about to be compacted.

Called by the PreCompact hook. Does NOT write files (Stop hook handles that).
Just informs the user and suggests options.`,
	RunE: runContextWarning,
}

func init() {
	rootCmd.AddCommand(contextWarningCmd)
}

func runContextWarning(cmd *cobra.Command, args []string) error {
	// Print to stderr (shown to user, not injected into context)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(os.Stderr, "  Context getting full — auto-DROPBAG saved")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  You can:")
	fmt.Fprintln(os.Stderr, "    - Keep going (older context will be compressed)")
	fmt.Fprintln(os.Stderr, "    - Run /drop to add your notes before continuing")
	fmt.Fprintln(os.Stderr, "    - Start fresh: clade resume (full context restored)")
	fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(os.Stderr)

	return nil
}
