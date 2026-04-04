package cmd

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var headStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Gracefully stop the head session",
	RunE:  runHeadStop,
}

func runHeadStop(cmd *cobra.Command, args []string) error {
	if !isHeadRunning() {
		ui.Info("Head session is not running")
		return nil
	}

	// Send Ctrl-C for graceful shutdown
	ui.Info("Stopping head session...")
	sendKeys := exec.Command("tmux", "send-keys", "-t", headTmuxSession, "C-c", "")
	if err := sendKeys.Run(); err != nil {
		ui.Warn("Failed to send interrupt signal: %v", err)
	}

	// Wait up to 10s for graceful shutdown
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !isHeadRunning() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill if still running
	if isHeadRunning() {
		ui.Warn("Graceful shutdown timed out, killing session...")
		killCmd := exec.Command("tmux", "kill-session", "-t", headTmuxSession)
		if err := killCmd.Run(); err != nil {
			return fmt.Errorf("failed to kill head session: %w", err)
		}
	}

	// Update session registry
	reg := session.NewRegistry(cladeBaseDir())
	if sess, err := reg.Get("head"); err == nil {
		sess.Status = session.StatusStopped
		sess.LastActive = time.Now()
		if err := reg.Save(sess); err != nil {
			ui.Warn("Failed to update session registry: %v", err)
		}
	}

	ui.Success("Head session stopped")
	return nil
}
