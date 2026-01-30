package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
var verboseFlag bool

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
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Wire up verbose flag to ui package
		ui.Verbose = verboseFlag
	},
	RunE: runRoot,
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

	// Verbose flag for debug output
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "V", false, "Enable verbose debug output")

	// Version flag
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "Print version and exit")

	// Common flags used by multiple commands (editor, agent control)
	// These are persistent so they're available to all subcommands
	rootCmd.PersistentFlags().StringVarP(&workEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	rootCmd.PersistentFlags().StringVarP(&workAgentFlag, "agent", "a", "", "Override configured agent")
	rootCmd.PersistentFlags().BoolVar(&workNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	rootCmd.PersistentFlags().BoolVar(&workNoEditorFlag, "no-editor", false, "Skip opening the editor")

	// Work-specific flags for the `clade foo` shorthand
	// These are local to root and will also be on work command
	rootCmd.Flags().StringVarP(&workTypeFlag, "type", "t", "", "Type of worktree (spike, feature, bug, chore, hotfix, docs)")
	rootCmd.Flags().StringVarP(&workRepoFlag, "repo", "r", "", "Repository path or registered name")
	rootCmd.Flags().BoolVarP(&workPickFlag, "pick", "p", false, "Force repo picker")
	rootCmd.Flags().StringVarP(&workBranchFlag, "branch", "b", "", "Custom branch name")
	rootCmd.Flags().BoolVar(&workDryRunFlag, "dry-run", false, "Preview what would be created without making changes")
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
		// Guard against typos of subcommands being silently treated as
		// worktree names (e.g. "clade injext-context" → creating a worktree
		// instead of erroring). Cobra's built-in "Did you mean?" is bypassed
		// because root has RunE set.
		if suggestion := suggestSubcommand(cmd, args[0]); suggestion != "" {
			return fmt.Errorf("unknown command %q\n\nDid you mean:\n  clade %s\n\nRun 'clade --help' for usage", args[0], suggestion)
		}
		return runWork(cmd, args)
	}

	// No args - show interactive dashboard
	return runInteractiveDashboard(cmd, args)
}

// suggestSubcommand checks if the given name is within edit distance 2 of any
// registered subcommand. Returns the closest match, or "" if none are close.
func suggestSubcommand(cmd *cobra.Command, name string) string {
	bestDist := 3 // threshold: only suggest if distance <= 2
	bestMatch := ""
	for _, sub := range cmd.Commands() {
		d := levenshtein(name, sub.Name())
		if d > 0 && d < bestDist {
			bestDist = d
			bestMatch = sub.Name()
		}
	}
	return bestMatch
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use single-row optimization: only need previous row + current row
	prev := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev = curr
	}
	return prev[lb]
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
	showDashboard(cfg, state)

	// Show action picker
	return showActionPicker(cfg, state)
}

func showDashboard(cfg *config.Config, state *config.State) {
	hasContent := false

	// Show worktrees grouped by repo - includes both tracked and untracked
	if hasWorktrees := showDashboardWorktrees(cfg, state); hasWorktrees {
		hasContent = true
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

// dashboardWorktree holds display info for a worktree in the dashboard
type dashboardWorktree struct {
	Name      string
	RepoName  string
	Tracked   *config.Worktree // nil if untracked
	Branch    string           // for untracked
	Orphaned  bool
}

// showDashboardWorktrees shows all worktrees (tracked + untracked) in the dashboard
// Returns true if any worktrees were displayed
func showDashboardWorktrees(cfg *config.Config, state *config.State) bool {
	// Collect all repos: registered + any with tracked worktrees
	repoSet := make(map[string]string) // name -> path

	// Add registered repos
	for name, path := range cfg.Repos {
		repoSet[name] = config.ExpandPath(path)
	}

	// Add repos from state that might not be registered anymore
	for repoName := range state.Worktrees {
		if _, ok := repoSet[repoName]; !ok {
			// Can't easily get repo path - skip unregistered repos in dashboard
			// They'll show in `clade list` with (unregistered repo) marker
		}
	}

	if len(repoSet) == 0 && len(state.Worktrees) == 0 {
		return false
	}

	// Collect all worktrees to display
	var allWorktrees []dashboardWorktree

	for repoName, repoPath := range repoSet {
		// Get actual git worktrees
		gitWorktrees, err := git.ListWorktreesDetailed(repoPath)
		if err != nil {
			// Can't query this repo, skip
			continue
		}

		// Get tracked worktrees from state
		trackedWorktrees := state.Worktrees[repoName]
		if trackedWorktrees == nil {
			trackedWorktrees = make(map[string]*config.Worktree)
		}

		seenTracked := make(map[string]bool)

		// First, check git worktrees
		for _, gwt := range gitWorktrees {
			if gwt.IsMain {
				continue // Skip main repo
			}

			// Try to match with tracked worktree
			var matched *config.Worktree
			var matchedName string
			for name, wt := range trackedWorktrees {
				wtPath := config.GetWorktreePath(cfg, repoName, wt)
				if wtPath == gwt.Path {
					matched = wt
					matchedName = name
					seenTracked[name] = true
					break
				}
			}

			if matched != nil {
				allWorktrees = append(allWorktrees, dashboardWorktree{
					Name:     matchedName,
					RepoName: repoName,
					Tracked:  matched,
				})
			} else {
				// Untracked - use folder name as display name
				folderName := filepath.Base(gwt.Path)
				allWorktrees = append(allWorktrees, dashboardWorktree{
					Name:     folderName,
					RepoName: repoName,
					Tracked:  nil,
					Branch:   gwt.Branch,
				})
			}
		}

		// Check for orphaned tracked worktrees (in state but not in git)
		for name, wt := range trackedWorktrees {
			if seenTracked[name] {
				continue
			}
			allWorktrees = append(allWorktrees, dashboardWorktree{
				Name:     name,
				RepoName: repoName,
				Tracked:  wt,
				Orphaned: true,
			})
		}
	}

	if len(allWorktrees) == 0 {
		return false
	}

	// Sort: tracked by last_used (descending), then untracked alphabetically
	sort.Slice(allWorktrees, func(i, j int) bool {
		// Tracked first
		if allWorktrees[i].Tracked != nil && allWorktrees[j].Tracked == nil {
			return true
		}
		if allWorktrees[i].Tracked == nil && allWorktrees[j].Tracked != nil {
			return false
		}
		// Both tracked: by last used
		if allWorktrees[i].Tracked != nil && allWorktrees[j].Tracked != nil {
			return allWorktrees[i].Tracked.LastUsed.After(allWorktrees[j].Tracked.LastUsed)
		}
		// Both untracked: alphabetically
		return allWorktrees[i].Name < allWorktrees[j].Name
	})

	ui.Header("Active worktrees:")

	// Show up to 5
	shown := 0
	for _, dw := range allWorktrees {
		if shown >= 5 {
			remaining := len(allWorktrees) - 5
			ui.Detail("%s", ui.Dim(fmt.Sprintf("  ... and %d more", remaining)))
			break
		}
		printDashboardWorktreeItem(dw)
		shown++
	}

	return true
}

// printDashboardWorktreeItem prints a single worktree in dashboard format
func printDashboardWorktreeItem(dw dashboardWorktree) {
	if dw.Orphaned {
		// Tracked but git worktree is gone
		fmt.Printf("  %s %s %s\n",
			ui.Cyan(dw.Name),
			ui.Dim("("+dw.RepoName+")"),
			ui.Red("(orphaned)"),
		)
		return
	}

	if dw.Tracked != nil {
		// Tracked worktree
		age := formatAge(dw.Tracked.LastUsed)

		staleMarker := ""
		if time.Since(dw.Tracked.LastUsed) > 7*24*time.Hour {
			staleMarker = " " + ui.Yellow("(stale)")
		}

		label := dw.Tracked.Label
		if label == "" || label == "worktree" {
			label = ""
		} else {
			label = "[" + label + "] "
		}

		fmt.Printf("  %s %s%s - %s%s\n",
			ui.Cyan(dw.Tracked.Name),
			ui.Dim(label),
			ui.Dim("("+dw.RepoName+")"),
			ui.Dim(age),
			staleMarker,
		)
	} else {
		// Untracked worktree
		fmt.Printf("  %s %s%s - %s\n",
			ui.Cyan(dw.Name),
			ui.Dim("(untracked) "),
			ui.Dim("("+dw.RepoName+")"),
			ui.Dim(dw.Branch),
		)
	}
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
			Handler: func() error {
				return runInteractiveNew(cfg)
			},
		},
		action{
			Name:        "Scratch",
			Description: "Create a no-git scratch folder",
			Handler:     runInteractiveScratch,
		},
		action{
			Name:        "List",
			Description: "Show all worktrees with full details",
			Handler: func() error {
				return runList(listCmd, []string{})
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

// runInteractiveNew prompts for repo selection, name, and optional type prefix
func runInteractiveNew(cfg *config.Config) error {
	// Step 1: Select repo (with smart default)
	repoName, repoPath, err := selectRepoInteractive(cfg)
	if err != nil {
		return nil // User cancelled or no repos
	}

	// Step 2: Ask for the worktree name
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

	// Step 3: Ask for branch prefix type
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

	// Map selection to type and label config
	types := []string{"", "spike", "feature", "bug", "chore"}
	selectedType := types[idx]

	// Determine label and branch prefix
	var label string
	var branchPrefix string
	if selectedType == "" {
		label = "worktree"
		branchPrefix = ""
	} else {
		labelCfg, ok := cfg.GetLabelConfig(selectedType)
		if !ok {
			return fmt.Errorf("unknown type '%s'", selectedType)
		}
		label = selectedType
		branchPrefix = labelCfg.BranchPrefix
	}

	// Step 4: Create worktree in selected repo
	// Update last repo before creating worktree
	cfg.LastRepo = repoPath
	if err := cfg.Save(); err != nil {
		ui.Warn("Failed to save config: %v", err)
	}

	return CreateWorktree(name, WorktreeConfig{
		Label:        label,
		BranchPrefix: branchPrefix,
	}, WorktreeOptions{
		RepoFlag:     repoName, // Use the selected repo
		EditorFlag:   workEditorFlag,
		AgentFlag:    workAgentFlag,
		NoAgentFlag:  workNoAgentFlag,
		NoEditorFlag: workNoEditorFlag,
		DryRunFlag:   workDryRunFlag,
	})
}

// selectRepoInteractive shows a repo picker with smart defaults
// Returns repo name, repo path, and error
func selectRepoInteractive(cfg *config.Config) (string, string, error) {
	if len(cfg.Repos) == 0 {
		ui.Warn("No repositories registered")
		ui.Detail("Register one first: clade repo add <path>")
		return "", "", fmt.Errorf("no repos")
	}

	// Detect if current directory is in a registered repo
	currentRepoName := detectCurrentRepo(cfg)

	// Build sorted list of repo names
	var repoNames []string
	for name := range cfg.Repos {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)

	// If only one repo, auto-select it
	if len(repoNames) == 1 {
		name := repoNames[0]
		path := config.ExpandPath(cfg.Repos[name])
		ui.Info("Using repo: %s", name)
		return name, path, nil
	}

	// Build display items with current repo marker
	var items []string
	defaultIdx := 0
	for i, name := range repoNames {
		path := cfg.Repos[name]
		marker := ""
		if name == currentRepoName {
			marker = " (current)"
			defaultIdx = i
		} else if config.ExpandPath(path) == cfg.LastRepo {
			marker = " (last used)"
		}
		items = append(items, fmt.Sprintf("%s  %s%s", name, ui.Dim(path), marker))
	}

	prompt := promptui.Select{
		Label:     "Select repository",
		Items:     items,
		CursorPos: defaultIdx,
		Size:      8,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return "", "", err
	}

	selectedName := repoNames[idx]
	selectedPath := config.ExpandPath(cfg.Repos[selectedName])
	return selectedName, selectedPath, nil
}

// detectCurrentRepo returns the name of the registered repo that contains cwd
// Returns empty string if cwd is not in a registered repo
func detectCurrentRepo(cfg *config.Config) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// If we're in a worktree, get the main repo
	if git.IsWorktree(cwd) {
		cwd, _ = git.GetMainRepoRoot(cwd)
	} else if git.IsGitRepo(cwd) {
		cwd, _ = git.GetRepoRoot(cwd)
	}

	// Check if cwd matches any registered repo
	for name, path := range cfg.Repos {
		expandedPath := config.ExpandPath(path)
		if expandedPath == cwd {
			return name
		}
	}

	return ""
}

// runInteractiveScratch prompts for name and creates a scratch folder
func runInteractiveScratch() error {
	prompt := promptui.Prompt{
		Label: "Scratch folder name",
	}
	name, err := prompt.Run()
	if err != nil {
		return nil // User cancelled
	}
	if name == "" {
		return fmt.Errorf("name is required")
	}

	return runScratch(scratchCmd, []string{name})
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

	// Sort by last used (descending) - use stdlib instead of bubble sort
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastUsed.After(result[j].LastUsed)
	})

	return result
}

func sortScratchesByLastUsed(scratches map[string]*config.Scratch) []*config.Scratch {
	result := make([]*config.Scratch, 0, len(scratches))
	for _, s := range scratches {
		result = append(result, s)
	}

	// Sort by last used (descending) - use stdlib instead of bubble sort
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastUsed.After(result[j].LastUsed)
	})

	return result
}
