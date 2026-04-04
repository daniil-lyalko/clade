package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var (
	headStartAttachFlag  bool
	headStartChannelFlag string
)

const headTmuxSession = "clade-head"

var headStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Launch claude --remote-control in a tmux session",
	RunE:  runHeadStart,
}

func init() {
	headStartCmd.Flags().BoolVar(&headStartAttachFlag, "attach", false, "Attach to tmux after starting")
	headStartCmd.Flags().StringVar(&headStartChannelFlag, "channel", "", "Channel plugin name to pass to claude")
}

func runHeadStart(cmd *cobra.Command, args []string) error {
	headDir := headDirectory()

	// Check head is initialized
	if _, err := os.Stat(headDir); os.IsNotExist(err) {
		return fmt.Errorf("head not initialized. Run `clade head init` first")
	}

	// Check if already running
	if isHeadRunning() {
		ui.Info("Head session is already running")
		ui.Detail("Attach with: clade head attach")
		if headStartAttachFlag {
			return attachToHead()
		}
		return nil
	}

	// Build claude command
	claudeCmd := "claude --remote-control"
	if headStartChannelFlag != "" {
		claudeCmd += fmt.Sprintf(" --channels plugin:%s@claude-plugins-official", headStartChannelFlag)
	}

	// Start tmux session
	tmuxCmd := exec.Command("tmux", "new-session", "-d", "-s", headTmuxSession,
		fmt.Sprintf("cd %s && %s", headDir, claudeCmd))
	if err := tmuxCmd.Run(); err != nil {
		return fmt.Errorf("failed to start head session: %w (is tmux installed?)", err)
	}

	// Register in session registry
	reg := session.NewRegistry(cladeBaseDir())
	sess := &session.Session{
		SessionID:  "head",
		Project:    "head",
		CWD:        headDir,
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     session.StatusActive,
		Summary:    "Orchestrator session",
	}
	if err := reg.Save(sess); err != nil {
		ui.Warn("Failed to register head session: %v", err)
	}

	ui.Success("Head session started")
	ui.Detail("Attach: clade head attach")
	ui.Detail("Stop:   clade head stop")
	ui.Detail("Status: clade head status")

	if headStartAttachFlag {
		return attachToHead()
	}

	return nil
}

// isHeadRunning checks if the clade-head tmux session exists.
func isHeadRunning() bool {
	cmd := exec.Command("tmux", "has-session", "-t", headTmuxSession)
	return cmd.Run() == nil
}

// attachToHead attaches to the head tmux session.
func attachToHead() error {
	cmd := exec.Command("tmux", "attach", "-t", headTmuxSession)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
