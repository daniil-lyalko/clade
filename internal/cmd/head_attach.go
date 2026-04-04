package cmd

import (
	"fmt"

	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var headAttachNameFlag string

var headAttachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Attach to the head tmux session",
	RunE:  runHeadAttach,
}

func init() {
	headAttachCmd.Flags().StringVar(&headAttachNameFlag, "name", "head", "Name of the head session to attach to")
}

func runHeadAttach(cmd *cobra.Command, args []string) error {
	name := headAttachNameFlag
	tmuxSession := headTmuxSessionName(name)

	if !isHeadRunningByName(tmuxSession) {
		ui.Error("Head session '%s' is not running", name)
		return fmt.Errorf("start it with: clade head start --name %s", name)
	}

	return attachToHeadByName(tmuxSession)
}
