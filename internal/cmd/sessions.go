package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	sessionsJSONFlag      bool
	sessionsActiveFlag    bool
	sessionsCleanFlag     bool
	sessionsNoInteractive bool
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Show all Claude Code sessions (dashboard)",
	Long: `Interactive dashboard showing all active, idle, and stale Claude Code sessions.

Pick a session to resume, or clean up stale ones.

Flags:
  --json            JSON output for scripting
  --active          Only show active/idle sessions
  --clean           Archive all stale sessions
  --no-interactive  Print dashboard and exit`,
	RunE: runSessions,
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
	sessionsCmd.Flags().BoolVar(&sessionsJSONFlag, "json", false, "Output as JSON")
	sessionsCmd.Flags().BoolVar(&sessionsActiveFlag, "active", false, "Only show active/idle sessions")
	sessionsCmd.Flags().BoolVar(&sessionsCleanFlag, "clean", false, "Archive all stale sessions")
	sessionsCmd.Flags().BoolVar(&sessionsNoInteractive, "no-interactive", false, "Print dashboard and exit")
}

func runSessions(cmd *cobra.Command, args []string) error {
	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)

	// Handle --clean
	if sessionsCleanFlag {
		archived, err := reg.ArchiveStale()
		if err != nil {
			return fmt.Errorf("failed to archive stale sessions: %w", err)
		}
		if archived > 0 {
			ui.Success("Archived %d stale session(s)", archived)
		} else {
			ui.Info("No stale sessions to archive")
		}

		// Also clean up old inbox files
		inbox := session.NewInbox(baseDir)
		removed, _ := inbox.Cleanup(30)
		if removed > 0 {
			ui.Success("Removed %d old inbox file(s)", removed)
		}
		return nil
	}

	sessions, err := reg.List()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Self-heal: mark "active" sessions that haven't been updated in 30min as stopped.
	// This catches subagents and crashed sessions whose Stop hook never fired.
	for _, sess := range sessions {
		if sess.Status == session.StatusActive && time.Since(sess.LastActive) > 30*time.Minute {
			sess.Status = session.StatusStopped
			if sess.Summary == "" {
				sess.Summary = "(no stop hook fired)"
			}
			reg.Save(sess)
		}
	}

	// Filter active-only
	if sessionsActiveFlag {
		var filtered []*session.Session
		for _, s := range sessions {
			if !s.IsStale() {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	// JSON output
	if sessionsJSONFlag {
		return formatSessionsJSON(os.Stdout, sessions)
	}

	if len(sessions) == 0 {
		ui.Info("No active sessions")
		ui.Detail("Start a Claude Code session and it will appear here automatically")
		return nil
	}

	// Print dashboard
	formatSessionsDashboard(os.Stdout, sessions)

	// Interactive mode (default unless --no-interactive)
	if sessionsNoInteractive {
		return nil
	}

	return interactiveSessionPicker(reg, sessions)
}

// formatSessionsDashboard writes the formatted session table to w.
func formatSessionsDashboard(w io.Writer, sessions []*session.Session) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-4s %-12s %-18s %-6s %s\n",
		"", "STATUS", "PROJECT", "AGE", "DOING")

	// Sort head sessions first
	sortedSessions := make([]*session.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.SessionID == "head" {
			sortedSessions = append([]*session.Session{s}, sortedSessions...)
		} else {
			sortedSessions = append(sortedSessions, s)
		}
	}

	for i, s := range sortedSessions {
		num := fmt.Sprintf("%d", i+1)
		status := s.StatusLabel()
		age := formatAgeShort(s.LastActive)

		summary := s.Summary
		if len(summary) > 50 {
			summary = summary[:50] + "..."
		}

		// Color the status indicator
		var statusColor string
		switch {
		case s.Status == session.StatusActive:
			statusColor = ui.Green(fmt.Sprintf("● %s", status))
		case s.IsStale():
			statusColor = ui.Red(fmt.Sprintf("● %s", status))
		default:
			statusColor = ui.Yellow(fmt.Sprintf("● %s", status))
		}

		// Add [HEAD] label for the orchestrator session
		project := s.Project
		if s.SessionID == "head" {
			project = ui.Magenta("[HEAD]") + " " + project
		}

		fmt.Fprintf(w, "  %-4s %-12s %-18s %-6s %s\n",
			num, statusColor, project, ui.Dim(age), summary)
	}

	active, idle, stale := countSessionsByStatus(sessions)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %d sessions (%d active, %d idle, %d stale)\n",
		len(sessions), active, idle, stale)
	fmt.Fprintln(w)
}

// formatSessionsJSON writes sessions as JSON array to w.
func formatSessionsJSON(w io.Writer, sessions []*session.Session) error {
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	fmt.Fprintln(w)
	return nil
}

// countSessionsByStatus counts active, idle (<24h), and stale (>24h) sessions.
func countSessionsByStatus(sessions []*session.Session) (active, idle, stale int) {
	for _, s := range sessions {
		switch {
		case s.Status == session.StatusActive:
			active++
		case s.IsStale():
			stale++
		default:
			idle++
		}
	}
	return
}

// interactiveSessionPicker shows the resume prompt.
func interactiveSessionPicker(reg *session.Registry, sessions []*session.Session) error {
	var items []string
	for i, s := range sessions {
		items = append(items, fmt.Sprintf("%d: %s (%s)", i+1, s.Project, s.StatusLabel()))
	}
	items = append(items, "c: Clean stale sessions")
	items = append(items, "q: Quit")

	prompt := promptui.Select{
		Label: "Pick an action",
		Items: items,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return nil // user cancelled
	}

	// Clean stale
	if idx == len(items)-2 {
		archived, err := reg.ArchiveStale()
		if err != nil {
			return err
		}
		ui.Success("Archived %d stale session(s)", archived)
		return nil
	}

	// Quit
	if idx == len(items)-1 {
		return nil
	}

	// Resume session
	if idx < len(sessions) {
		return resumeSession(sessions[idx])
	}

	return nil
}

// resumeSession launches `claude --resume <session_id>` in the session's CWD.
func resumeSession(sess *session.Session) error {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found: %w", err)
	}

	ui.Info("Resuming session in %s", sess.CWD)

	cmd := exec.Command(claudePath, "--resume", sess.SessionID)
	cmd.Dir = sess.CWD
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
