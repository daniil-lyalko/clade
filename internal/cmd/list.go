package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all active worktrees, projects, and scratches",
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

	// List v2 worktrees (grouped by repo)
	if len(state.Worktrees) > 0 {
		hasContent = true
		printWorktreesGrouped(cfg, state)
	}

	// List v1 experiments (legacy format)
	if len(state.Experiments) > 0 {
		hasContent = true
		ui.Header("Experiments (legacy):")
		for _, exp := range state.Experiments {
			printExperiment(exp)
		}
		fmt.Println()
	}

	// List projects
	if len(state.Projects) > 0 {
		hasContent = true
		ui.Header("Projects")
		for _, proj := range state.Projects {
			printProject(proj)
		}
		fmt.Println()
	}

	// List scratches
	if len(state.Scratches) > 0 {
		hasContent = true
		ui.Header("Scratch")
		for _, scratch := range state.Scratches {
			printScratch(scratch)
		}
		fmt.Println()
	}

	if !hasContent {
		ui.Info("No active worktrees, projects, or scratch folders")
		ui.Detail("Create one with: clade feature <name>")
		ui.Detail("Or for bugs: clade bug <name>")
		ui.Detail("Or for spikes: clade spike <name>")
		ui.Detail("Or for no-git: clade scratch <name>")
	}

	return nil
}

// printWorktreesGrouped prints worktrees grouped by repo in a compact format
func printWorktreesGrouped(cfg *config.Config, state *config.State) {
	// Sort repo names
	var repoNames []string
	for repoName := range state.Worktrees {
		repoNames = append(repoNames, repoName)
	}
	sort.Strings(repoNames)

	for _, repoName := range repoNames {
		worktrees := state.Worktrees[repoName]
		count := len(worktrees)

		// Print repo header
		fmt.Printf("%s (%d worktree", ui.Cyan(repoName), count)
		if count != 1 {
			fmt.Print("s")
		}
		fmt.Println(")")

		// Sort worktrees by last used (most recent first)
		var wtList []*config.Worktree
		for _, wt := range worktrees {
			wtList = append(wtList, wt)
		}
		sort.Slice(wtList, func(i, j int) bool {
			return wtList[i].LastUsed.After(wtList[j].LastUsed)
		})

		// Print each worktree in compact format
		for _, wt := range wtList {
			printWorktreeCompact(cfg, repoName, wt)
		}
		fmt.Println()
	}
}

// printWorktreeCompact prints a worktree in compact single-line format
func printWorktreeCompact(cfg *config.Config, repoName string, wt *config.Worktree) {
	wtPath := config.WorktreePath(cfg, repoName, wt.Name)
	age := formatAgeShort(wt.LastUsed)

	// Check if stale (older than 7 days)
	staleMarker := ""
	if time.Since(wt.LastUsed) > 7*24*time.Hour {
		staleMarker = ui.Yellow("(stale)")
	}

	// Check for uncommitted changes
	statusMarker := ""
	if hasChanges, _ := git.HasUncommittedChanges(wtPath); hasChanges {
		statusMarker = ui.Yellow("*")
	}

	// Format: "  name          label         age  (stale)"
	fmt.Printf("  %-16s %-12s %s %s%s\n",
		wt.Name+statusMarker,
		ui.Dim(wt.Label),
		ui.Dim(age),
		staleMarker,
		"",
	)
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
		staleMarker = " " + ui.Yellow("(stale)")
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

// formatAgeShort returns a compact age string like "2h" or "3d"
func formatAgeShort(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
