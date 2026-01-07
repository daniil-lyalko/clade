package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "clade",
	Short: "Claude Code Workflow CLI",
	Long: `Clade manages git worktrees and context for AI coding sessions.

Named after biological clades (branching groups sharing common ancestry) -
perfect metaphor for worktree branches.

Quick start:
  clade exp try-redis       # Create isolated experiment
  clade list                # See what's active
  clade resume try-redis    # Get back to work
  clade cleanup try-redis   # Clean up when done`,
	RunE: runInteractiveDashboard,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags can be added here
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
}

// runInteractiveDashboard shows a dashboard and action picker when clade is run with no args
func runInteractiveDashboard(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Show dashboard
	showDashboard(state)

	// Show action picker
	return showActionPicker(cfg, state)
}

func showDashboard(state *config.State) {
	hasContent := false

	// Show experiments (most recent first, limit to 5)
	if len(state.Experiments) > 0 {
		hasContent = true
		ui.Header("Active experiments:")
		exps := sortExperimentsByLastUsed(state.Experiments)
		shown := 0
		for _, exp := range exps {
			if shown >= 5 {
				remaining := len(exps) - 5
				ui.Detail("%s", ui.Dim(fmt.Sprintf("  ... and %d more", remaining)))
				break
			}
			printDashboardExperiment(exp)
			shown++
		}
	}

	// Show projects
	if len(state.Projects) > 0 {
		hasContent = true
		ui.Header("Active projects:")
		for _, proj := range state.Projects {
			printDashboardProject(proj)
		}
	}

	// Show scratches (limit to 3)
	if len(state.Scratches) > 0 {
		hasContent = true
		ui.Header("Scratch folders:")
		scratches := sortScratchesByLastUsed(state.Scratches)
		shown := 0
		for _, scratch := range scratches {
			if shown >= 3 {
				remaining := len(scratches) - 3
				ui.Detail("%s", ui.Dim(fmt.Sprintf("  ... and %d more", remaining)))
				break
			}
			printDashboardScratch(scratch)
			shown++
		}
	}

	if !hasContent {
		fmt.Println()
		ui.Info("No active experiments, projects, or scratch folders")
	}

	fmt.Println()
}

func printDashboardExperiment(exp *config.Experiment) {
	repoName := filepath.Base(exp.Repo)
	age := formatAge(exp.LastUsed)

	// Check if stale
	staleMarker := ""
	if time.Since(exp.LastUsed) > 7*24*time.Hour {
		staleMarker = " " + ui.Yellow("(stale)")
	}

	// Check for uncommitted changes
	statusMarker := ""
	if hasChanges, _ := git.HasUncommittedChanges(exp.Path); hasChanges {
		statusMarker = " " + ui.Yellow("*")
	}

	fmt.Printf("  %s %s - %s%s%s\n",
		ui.Cyan(exp.Name),
		ui.Dim("("+repoName+")"),
		ui.Dim(age),
		staleMarker,
		statusMarker,
	)
}

func printDashboardProject(proj *config.Project) {
	age := formatAge(proj.LastUsed)

	var repoNames []string
	for _, r := range proj.Repos {
		repoNames = append(repoNames, r.Name)
	}

	fmt.Printf("  %s - %s\n",
		ui.Cyan(proj.Name),
		ui.Dim(age),
	)
}

func printDashboardScratch(scratch *config.Scratch) {
	age := formatAge(scratch.LastUsed)

	staleMarker := ""
	if time.Since(scratch.LastUsed) > 7*24*time.Hour {
		staleMarker = " " + ui.Yellow("(stale)")
	}

	fmt.Printf("  %s %s - %s%s\n",
		ui.Cyan(scratch.Name),
		ui.Dim("(no-git)"),
		ui.Dim(age),
		staleMarker,
	)
}

func showActionPicker(cfg *config.Config, state *config.State) error {
	type action struct {
		Name        string
		Description string
		Handler     func() error
	}

	actions := []action{}

	// Only show "Resume" if there's something to resume
	hasItems := len(state.Experiments) > 0 || len(state.Projects) > 0 || len(state.Scratches) > 0
	if hasItems {
		actions = append(actions, action{
			Name:        "Resume",
			Description: "Resume an experiment, project, or scratch",
			Handler: func() error {
				return resumeInteractive(cfg, state)
			},
		})
	}

	actions = append(actions,
		action{
			Name:        "New experiment",
			Description: "Create an isolated worktree experiment",
			Handler: func() error {
				return runExp(expCmd, []string{})
			},
		},
		action{
			Name:        "New project",
			Description: "Create a multi-repo workspace",
			Handler: func() error {
				return runProject(projectCmd, []string{})
			},
		},
		action{
			Name:        "New scratch",
			Description: "Create a no-git scratch folder",
			Handler: func() error {
				return runScratch(scratchCmd, []string{})
			},
		},
		action{
			Name:        "Register repo",
			Description: "Add a repository for quick access",
			Handler:     runInteractiveRepoAdd,
		},
	)

	// Only show cleanup if there's something to clean
	if hasItems {
		actions = append(actions, action{
			Name:        "Clean up",
			Description: "Remove an experiment, project, or scratch",
			Handler: func() error {
				return runCleanup(cleanupCmd, []string{})
			},
		})
	}

	actions = append(actions, action{
		Name:        "View all",
		Description: "Show detailed list of all items",
		Handler: func() error {
			return runList(listCmd, []string{})
		},
	})

	actions = append(actions, action{
		Name:        "Exit",
		Description: "",
		Handler:     func() error { return nil },
	})

	// Build display items
	var items []string
	for _, a := range actions {
		if a.Description != "" {
			items = append(items, fmt.Sprintf("%s  %s", a.Name, ui.Dim(a.Description)))
		} else {
			items = append(items, a.Name)
		}
	}

	prompt := promptui.Select{
		Label: "What would you like to do",
		Items: items,
		Size:  10,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		// User cancelled (Ctrl+C)
		return nil
	}

	return actions[idx].Handler()
}

// runInteractiveRepoAdd prompts for a path and adds a repo
func runInteractiveRepoAdd() error {
	prompt := promptui.Prompt{
		Label:   "Repository path",
		Default: ".",
	}

	path, err := prompt.Run()
	if err != nil {
		return nil
	}

	return runRepoAdd(repoAddCmd, []string{path})
}

func sortExperimentsByLastUsed(exps map[string]*config.Experiment) []*config.Experiment {
	result := make([]*config.Experiment, 0, len(exps))
	for _, exp := range exps {
		result = append(result, exp)
	}

	// Bubble sort by last used (descending)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].LastUsed.After(result[i].LastUsed) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

func sortScratchesByLastUsed(scratches map[string]*config.Scratch) []*config.Scratch {
	result := make([]*config.Scratch, 0, len(scratches))
	for _, s := range scratches {
		result = append(result, s)
	}

	// Bubble sort by last used (descending)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].LastUsed.After(result[i].LastUsed) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}
