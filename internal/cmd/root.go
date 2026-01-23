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

// Version is set at build time via -ldflags
var Version = "dev"

var experimentalFlag bool
var versionFlag bool

var rootCmd = &cobra.Command{
	Use:   "clade [name]",
	Short: "Claude Code Workflow CLI",
	Long: `Clade manages git worktrees and context for AI coding sessions.

Named after biological clades (branching groups sharing common ancestry) -
perfect metaphor for worktree branches.

Quick start:
  clade foo                       # Create worktree (branch: foo)
  clade foo -t spike              # Create spike worktree (branch: spike/foo)
  clade list                      # See what's active
  clade resume foo                # Get back to work
  clade cleanup foo               # Clean up when done

Shortcut:
  clade <name>                    # Same as: clade work <name>`,
	Args:                  cobra.ArbitraryArgs,
	DisableFlagsInUseLine: true,
	RunE:                  runRoot,
}

// IsExperimentalEnabled returns true if --experimental flag is set
func IsExperimentalEnabled() bool {
	return experimentalFlag
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global experimental flag for hidden features
	rootCmd.PersistentFlags().BoolVar(&experimentalFlag, "experimental", false, "Enable experimental features (project command)")

	// Version flag
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "Print version and exit")

	// Add work command flags to root so `clade foo -t spike` works
	rootCmd.Flags().StringVarP(&workTypeFlag, "type", "t", "", "Type of worktree (feature, bug, spike, chore, hotfix, docs, or custom)")
	rootCmd.Flags().StringVarP(&workRepoFlag, "repo", "r", "", "Repository path or registered name")
	rootCmd.Flags().BoolVarP(&workPickFlag, "pick", "p", false, "Force repo picker")
	rootCmd.Flags().StringVarP(&workBranchFlag, "branch", "b", "", "Custom branch name")
	rootCmd.Flags().StringVarP(&workEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	rootCmd.Flags().StringVarP(&workAgentFlag, "agent", "a", "", "Override configured agent")
	rootCmd.Flags().BoolVar(&workNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	rootCmd.Flags().BoolVar(&workNoEditorFlag, "no-editor", false, "Skip opening the editor")
}

// runRoot handles the root command - either delegates to work or shows interactive dashboard
func runRoot(cmd *cobra.Command, args []string) error {
	// Handle --version flag
	if versionFlag {
		fmt.Printf("clade %s\n", Version)
		return nil
	}

	// If a name is provided, delegate to work command
	if len(args) > 0 {
		return runWork(cmd, args)
	}

	// No args - show interactive dashboard
	return runInteractiveDashboard(cmd, args)
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

	// Show worktrees grouped by repo (v2 format - most recent first, limit to 5)
	if len(state.Worktrees) > 0 {
		hasContent = true
		ui.Header("Active worktrees:")
		worktrees := sortWorktreesByLastUsed(state.Worktrees)
		shown := 0
		for _, wt := range worktrees {
			if shown >= 5 {
				total := countTotalWorktrees(state.Worktrees)
				remaining := total - 5
				ui.Detail("%s", ui.Dim(fmt.Sprintf("  ... and %d more", remaining)))
				break
			}
			printDashboardWorktree(wt)
			shown++
		}
	}

	// Show legacy experiments (most recent first, limit to 3)
	if len(state.Experiments) > 0 {
		hasContent = true
		ui.Header("Legacy experiments:")
		exps := sortExperimentsByLastUsed(state.Experiments)
		shown := 0
		for _, exp := range exps {
			if shown >= 3 {
				remaining := len(exps) - 3
				ui.Detail("%s", ui.Dim(fmt.Sprintf("  ... and %d more", remaining)))
				break
			}
			printDashboardExperiment(exp)
			shown++
		}
	}

	// Show projects (only if experimental or they exist)
	if len(state.Projects) > 0 {
		hasContent = true
		ui.Header("Projects:")
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
		ui.Info("No active worktrees")
		ui.Detail("Create one with: clade <name>")
	}

	fmt.Println()
}

// worktreeWithRepo holds a worktree and its repo name for sorting
type worktreeWithRepo struct {
	RepoName string
	Worktree *config.Worktree
}

func printDashboardWorktree(wt worktreeWithRepo) {
	age := formatAge(wt.Worktree.LastUsed)

	// Check if stale
	staleMarker := ""
	if time.Since(wt.Worktree.LastUsed) > 7*24*time.Hour {
		staleMarker = " " + ui.Yellow("(stale)")
	}

	label := wt.Worktree.Label
	if label == "" || label == "worktree" {
		label = ""
	} else {
		label = "[" + label + "] "
	}

	fmt.Printf("  %s %s%s - %s%s\n",
		ui.Cyan(wt.Worktree.Name),
		ui.Dim(label),
		ui.Dim("("+wt.RepoName+")"),
		ui.Dim(age),
		staleMarker,
	)
}

func sortWorktreesByLastUsed(worktrees map[string]map[string]*config.Worktree) []worktreeWithRepo {
	var result []worktreeWithRepo
	for repoName, repoWorktrees := range worktrees {
		for _, wt := range repoWorktrees {
			result = append(result, worktreeWithRepo{
				RepoName: repoName,
				Worktree: wt,
			})
		}
	}

	// Sort by last used (descending)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Worktree.LastUsed.After(result[i].Worktree.LastUsed) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

func countTotalWorktrees(worktrees map[string]map[string]*config.Worktree) int {
	count := 0
	for _, repoWorktrees := range worktrees {
		count += len(repoWorktrees)
	}
	return count
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
	hasItems := len(state.Worktrees) > 0 || len(state.Experiments) > 0 || len(state.Projects) > 0 || len(state.Scratches) > 0
	if hasItems {
		actions = append(actions, action{
			Name:        "Resume",
			Description: "Continue working on an existing worktree",
			Handler: func() error {
				return resumeInteractive(cfg, state)
			},
		})
	}

	actions = append(actions,
		action{
			Name:        "New",
			Description: "Create a new worktree",
			Handler:     runInteractiveNew,
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
			Description: "Remove a worktree or scratch folder",
			Handler: func() error {
				return runCleanup(cleanupCmd, []string{})
			},
		})
	}

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
		Size:  8,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		// User cancelled (Ctrl+C)
		return nil
	}

	return actions[idx].Handler()
}

// runInteractiveNew prompts for name and optional type prefix
func runInteractiveNew() error {
	// First ask for the name
	namePrompt := promptui.Prompt{
		Label: "Worktree name",
	}
	name, err := namePrompt.Run()
	if err != nil {
		return nil // User cancelled
	}
	if name == "" {
		return fmt.Errorf("name is required")
	}

	// Then ask if they want a branch prefix
	typePrompt := promptui.Select{
		Label: "Branch prefix",
		Items: []string{
			fmt.Sprintf("None  %s", ui.Dim("branch: "+name)),
			fmt.Sprintf("spike  %s", ui.Dim("branch: spike/"+name)),
			fmt.Sprintf("feature  %s", ui.Dim("branch: feat/"+name)),
			fmt.Sprintf("bug  %s", ui.Dim("branch: fix/"+name)),
			fmt.Sprintf("chore  %s", ui.Dim("branch: chore/"+name)),
		},
		Size: 5,
	}

	idx, _, err := typePrompt.Run()
	if err != nil {
		return nil // User cancelled
	}

	// Map selection to type flag
	types := []string{"", "spike", "feature", "bug", "chore"}
	workTypeFlag = types[idx]
	defer func() { workTypeFlag = "" }()

	return runWork(workCmd, []string{name})
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
