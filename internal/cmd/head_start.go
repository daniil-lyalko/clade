package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
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
	headStartCmd.Flags().StringVar(&headStartNameFlag, "name", session.HeadSessionID, "Name for the head session (tmux session: clade-{name})")
}

func runHeadStart(cmd *cobra.Command, args []string) error {
	name := headStartNameFlag
	tmuxSession := headTmuxSessionName(name)
	headDir := headDirectory()

	// Check head is initialized
	if _, err := os.Stat(headDir); os.IsNotExist(err) {
		return fmt.Errorf("head not initialized. Run `clade head init` first")
	}

	// Check if already running — if so, just go to it
	if isHeadRunningByName(tmuxSession) {
		return goToHead(tmuxSession, name)
	}

	// Build claude command
	claudeCmd := "claude --remote-control"
	if headStartChannelFlag != "" {
		quotedChannel := "'" + strings.ReplaceAll(headStartChannelFlag, "'", "'\"'\"'") + "'"
		claudeCmd += fmt.Sprintf(" --channels plugin:%s@claude-plugins-official", quotedChannel)
	}

	// Quote headDir for safe shell interpolation
	quotedDir := "'" + strings.ReplaceAll(headDir, "'", "'\"'\"'") + "'"
	shellCmd := fmt.Sprintf("cd %s && %s", quotedDir, claudeCmd)

	inTmux := os.Getenv("TMUX") != ""

	if inTmux {
		// Inside tmux: create a new window in current session
		tmuxCmd := exec.Command("tmux", "new-window", "-n", tmuxSession, shellCmd)
		if err := tmuxCmd.Run(); err != nil {
			return fmt.Errorf("failed to create tmux window: %w", err)
		}
	} else {
		// Not in tmux: create a new tmux session and attach
		tmuxCmd := exec.Command("tmux", "new-session", "-s", tmuxSession, shellCmd)
		tmuxCmd.Stdin = os.Stdin
		tmuxCmd.Stdout = os.Stdout
		tmuxCmd.Stderr = os.Stderr

		// Register before attaching (attach blocks)
		registerHeadSession(name, headDir)

		return tmuxCmd.Run()
	}

	// Register in session registry
	registerHeadSession(name, headDir)

	ui.Success("Head session '%s' started", name)
	ui.Detail("Switch to it: Ctrl-a then select '%s' window", tmuxSession)

	return nil
}

// registerHeadSession saves the head session to the Clade registry.
func registerHeadSession(name, headDir string) {
	reg := session.NewRegistry(config.DotCladeDir())
	sess := &session.Session{
		SessionID:  name,
		Project:    session.HeadSessionID,
		CWD:        headDir,
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     session.StatusActive,
		Summary:    "Orchestrator session",
	}
	if err := reg.Save(sess); err != nil {
		ui.Warn("Failed to register head session: %v", err)
	}
}

// goToHead switches to an already-running head session.
func goToHead(tmuxSession, name string) error {
	inTmux := os.Getenv("TMUX") != ""
	if inTmux {
		// Try switching to the window in current tmux session
		if exec.Command("tmux", "select-window", "-t", tmuxSession).Run() == nil {
			return nil
		}
		// Window might be in a different tmux session, switch client
		if exec.Command("tmux", "switch-client", "-t", tmuxSession).Run() == nil {
			return nil
		}
	}
	// Fallback: attach
	ui.Info("Head '%s' is running. Attaching...", name)
	return attachToHeadByName(tmuxSession)
}

// isHeadRunning checks if the default clade-head tmux session exists.
func isHeadRunning() bool {
	return isHeadRunningByName(headTmuxSessionName(session.HeadSessionID))
}

// isHeadRunningByName checks if a tmux session with the given name exists.
func isHeadRunningByName(tmuxSession string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", tmuxSession)
	return cmd.Run() == nil
}

// attachToHead attaches to the default head tmux session.
func attachToHead() error {
	return attachToHeadByName(headTmuxSessionName(session.HeadSessionID))
}

// attachToHeadByName switches to or attaches to a tmux session/window.
func attachToHeadByName(tmuxSession string) error {
	inTmux := os.Getenv("TMUX") != ""
	if inTmux {
		// Try window switch first
		if exec.Command("tmux", "select-window", "-t", tmuxSession).Run() == nil {
			return nil
		}
		// Try client switch (different tmux session)
		if exec.Command("tmux", "switch-client", "-t", tmuxSession).Run() == nil {
			return nil
		}
	}
	// Not in tmux or switch failed: regular attach
	cmd := exec.Command("tmux", "attach", "-t", tmuxSession)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
