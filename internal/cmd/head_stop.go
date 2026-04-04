package cmd

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

const gracefulShutdownTimeout = 10 * time.Second

var headStopNameFlag string

var headStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Gracefully stop the head session",
	RunE:  runHeadStop,
}

func init() {
	headStopCmd.Flags().StringVar(&headStopNameFlag, "name", session.HeadSessionID, "Name of the head session to stop")
}

func runHeadStop(cmd *cobra.Command, args []string) error {
	name := headStopNameFlag
	tmuxSession := headTmuxSessionName(name)

	if !isHeadRunningByName(tmuxSession) {
		ui.Info("Head session '%s' is not running", name)
		return nil
	}

	// Send Ctrl-C for graceful shutdown
	ui.Info("Stopping head session '%s'...", name)
	sendKeys := exec.Command("tmux", "send-keys", "-t", tmuxSession, "C-c", "")
	if err := sendKeys.Run(); err != nil {
		ui.Warn("Failed to send interrupt signal: %v", err)
	}

	// Wait up to 10s for graceful shutdown
	deadline := time.Now().Add(gracefulShutdownTimeout)
	for time.Now().Before(deadline) {
		if !isHeadRunningByName(tmuxSession) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill if still running
	if isHeadRunningByName(tmuxSession) {
		ui.Warn("Graceful shutdown timed out, killing session...")
		killCmd := exec.Command("tmux", "kill-session", "-t", tmuxSession)
		if err := killCmd.Run(); err != nil {
			return fmt.Errorf("failed to kill head session: %w", err)
		}
	}

	// Update session registry
	reg := session.NewRegistry(config.DotCladeDir())
	if sess, err := reg.Get(name); err == nil {
		sess.Status = session.StatusStopped
		sess.LastActive = time.Now()
		if err := reg.Save(sess); err != nil {
			ui.Warn("Failed to update session registry: %v", err)
		}
	}

	ui.Success("Head session '%s' stopped", name)
	return nil
}
