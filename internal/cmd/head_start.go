package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var (
	headStartAttachFlag  bool
	headStartChannelFlag string
	headStartNameFlag    string
)

// headTmuxSessionName returns the tmux session name for the given head name.
func headTmuxSessionName(name string) string {
	return "clade-" + name
}

var headStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Launch claude --remote in a tmux session",
	RunE:  runHeadStart,
}

func init() {
	headStartCmd.Flags().BoolVar(&headStartAttachFlag, "attach", false, "Attach to tmux after starting")
	headStartCmd.Flags().StringVar(&headStartChannelFlag, "channel", "", "Channel plugin name to pass to claude")
	headStartCmd.Flags().StringVar(&headStartNameFlag, "name", "head", "Name for the head session (tmux session: clade-{name})")
}

func runHeadStart(cmd *cobra.Command, args []string) error {
	name := headStartNameFlag
	tmuxSession := headTmuxSessionName(name)
	headDir := headDirectory()

	// Check head is initialized
	if _, err := os.Stat(headDir); os.IsNotExist(err) {
		return fmt.Errorf("head not initialized. Run `clade head init` first")
	}

	// Check if already running
	if isHeadRunningByName(tmuxSession) {
		ui.Info("Head session '%s' is already running", name)
		ui.Detail("Attach with: clade head attach --name %s", name)
		if headStartAttachFlag {
			return attachToHeadByName(tmuxSession)
		}
		return nil
	}

	// Build claude command
	claudeCmd := "claude --remote"
	if headStartChannelFlag != "" {
		claudeCmd += fmt.Sprintf(" --channels plugin:%s@claude-plugins-official", headStartChannelFlag)
	}

	// Quote headDir for safe shell interpolation (single quotes, escape internal single quotes)
	quotedDir := "'" + strings.ReplaceAll(headDir, "'", "'\"'\"'") + "'"

	// Start tmux session
	tmuxCmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxSession,
		fmt.Sprintf("cd %s && %s", quotedDir, claudeCmd))
	if err := tmuxCmd.Run(); err != nil {
		return fmt.Errorf("failed to start head session: %w (is tmux installed?)", err)
	}

	// Register in session registry
	reg := session.NewRegistry(cladeBaseDir())
	sess := &session.Session{
		SessionID:  name,
		Project:    "head",
		CWD:        headDir,
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     session.StatusActive,
		Summary:    fmt.Sprintf("Orchestrator session (%s)", name),
	}
	if err := reg.Save(sess); err != nil {
		ui.Warn("Failed to register head session: %v", err)
	}

	ui.Success("Head session '%s' started", name)
	ui.Detail("Attach: clade head attach --name %s", name)
	ui.Detail("Stop:   clade head stop --name %s", name)
	ui.Detail("Status: clade head status --name %s", name)

	if headStartAttachFlag {
		return attachToHeadByName(tmuxSession)
	}

	return nil
}

// isHeadRunning checks if the default clade-head tmux session exists.
func isHeadRunning() bool {
	return isHeadRunningByName(headTmuxSessionName("head"))
}

// isHeadRunningByName checks if a tmux session with the given name exists.
func isHeadRunningByName(tmuxSession string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", tmuxSession)
	return cmd.Run() == nil
}

// attachToHead attaches to the default head tmux session.
func attachToHead() error {
	return attachToHeadByName(headTmuxSessionName("head"))
}

// attachToHeadByName attaches to a tmux session by name.
func attachToHeadByName(tmuxSession string) error {
	cmd := exec.Command("tmux", "attach", "-t", tmuxSession)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
