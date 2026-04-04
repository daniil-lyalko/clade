package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var headStatusNameFlag string

var headStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running state and info",
	RunE:  runHeadStatus,
}

func init() {
	headStatusCmd.Flags().StringVar(&headStatusNameFlag, "name", session.HeadSessionID, "Name of the head session to check")
}

func runHeadStatus(cmd *cobra.Command, args []string) error {
	name := headStatusNameFlag
	tmuxSession := headTmuxSessionName(name)
	headDir := headDirectory()

	// Check initialization
	if _, err := os.Stat(headDir); os.IsNotExist(err) {
		ui.Info("Head not initialized. Run `clade head init` first")
		return nil
	}

	running := isHeadRunningByName(tmuxSession)

	fmt.Println()
	ui.KeyValue("Name", name)
	if running {
		ui.KeyValue("Status", ui.Green("running"))
	} else {
		ui.KeyValue("Status", ui.Yellow("stopped"))
	}

	// Show session info from registry
	reg := session.NewRegistry(config.DotCladeDir())
	if sess, err := reg.Get(name); err == nil {
		if running {
			uptime := time.Since(sess.Started)
			ui.KeyValue("Uptime", formatDurationHuman(uptime))
		}
		ui.KeyValue("Last active", formatTimeAgo(sess.LastActive))
	}

	// Check brain
	brainPath := filepath.Join(headDir, "brain")
	if target, err := os.Readlink(brainPath); err == nil {
		ui.KeyValue("Brain", target)
	} else if _, err := os.Stat(brainPath); err == nil {
		ui.KeyValue("Brain", brainPath)
	} else {
		ui.KeyValue("Brain", ui.Dim("not configured"))
	}

	// Count skills
	skillsDir := filepath.Join(headDir, ".claude", "skills")
	skillCount := 0
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				skillCount++
			}
		}
	}
	ui.KeyValue("Skills", fmt.Sprintf("%d", skillCount))
	fmt.Println()

	return nil
}

// formatDurationHuman formats a duration in a human-readable way.
func formatDurationHuman(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	if h > 0 {
		return fmt.Sprintf("%dd %dh", days, h)
	}
	return fmt.Sprintf("%dd", days)
}

// formatTimeAgo formats a time as "X ago".
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	return formatDurationHuman(d) + " ago"
}
