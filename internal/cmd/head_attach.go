package cmd

import (
	"fmt"

	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var headAttachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Attach to the head tmux session",
	RunE:  runHeadAttach,
}

func runHeadAttach(cmd *cobra.Command, args []string) error {
	if !isHeadRunning() {
		ui.Error("Head session is not running")
		return fmt.Errorf("start it with: clade head start")
	}

	return attachToHead()
}
