package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all active worktrees, experiments, projects",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	hasContent := false

	// List experiments
	if len(state.Experiments) > 0 {
		hasContent = true
		ui.Header("Experiments:")
		for _, exp := range state.Experiments {
			printExperiment(exp)
		}
	}

	// List projects
	if len(state.Projects) > 0 {
		hasContent = true
		ui.Header("Projects:")
		for _, proj := range state.Projects {
			printProject(proj)
		}
	}

	// List scratches
	if len(state.Scratches) > 0 {
		hasContent = true
		ui.Header("Scratch:")
		for _, scratch := range state.Scratches {
			printScratch(scratch)
		}
	}

	if !hasContent {
		ui.Info("No active experiments, projects, or scratch folders")
		ui.Detail("Create one with: clade exp <name>")
		ui.Detail("Or for no-git: clade scratch <name>")
	}

	return nil
}

func printExperiment(exp *config.Experiment) {
	repoName := filepath.Base(exp.Repo)
	age := formatAge(exp.LastUsed)

	// Check status
	status := ""
	if hasChanges, _ := git.HasUncommittedChanges(exp.Path); hasChanges {
		status = ui.Yellow("uncommitted changes")
	} else {
		status = ui.Green("clean")
	}

	// Check if stale (older than 7 days)
	staleMarker := ""
	if time.Since(exp.LastUsed) > 7*24*time.Hour {
		staleMarker = " " + ui.Yellow("⚠")
	}

	fmt.Printf("  %s %s%s\n", ui.Cyan(exp.Name), ui.Dim("("+repoName+")"), staleMarker)
	ui.KeyValue("Branch", exp.Branch)
	ui.KeyValue("Path", exp.Path)
	ui.KeyValue("Age", age)
	ui.KeyValue("Status", status)
	if exp.Ticket != "" {
		ui.KeyValue("Ticket", exp.Ticket)
	}
	fmt.Println()
}

func printProject(proj *config.Project) {
	age := formatAge(proj.LastUsed)

	var repoNames []string
	for _, r := range proj.Repos {
		repoNames = append(repoNames, r.Name)
	}

	fmt.Printf("  %s\n", ui.Cyan(proj.Name))
	ui.KeyValue("Branch", proj.Branch)
	ui.KeyValue("Path", proj.Path)
	ui.KeyValue("Repos", fmt.Sprintf("%v", repoNames))
	ui.KeyValue("Age", age)
	fmt.Println()
}

func printScratch(scratch *config.Scratch) {
	age := formatAge(scratch.LastUsed)

	// Check if stale (older than 7 days)
	staleMarker := ""
	if time.Since(scratch.LastUsed) > 7*24*time.Hour {
		staleMarker = " " + ui.Yellow("⚠")
	}

	fmt.Printf("  %s %s%s\n", ui.Cyan(scratch.Name), ui.Dim("(no-git)"), staleMarker)
	ui.KeyValue("Path", scratch.Path)
	ui.KeyValue("Age", age)
	if scratch.Ticket != "" {
		ui.KeyValue("Ticket", scratch.Ticket)
	}
	fmt.Println()
}

func formatAge(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
