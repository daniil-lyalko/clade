package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/hooks"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	resumeRepoFlag     string
	resumeEditorFlag   string
	resumeBranchFlag   string
	resumeAgentFlag    string
	resumeNoAgentFlag  bool
	resumeNoEditorFlag bool
)

var resumeCmd = &cobra.Command{
	Use:   "resume [name]",
	Short: "Resume an experiment or project",
	Long: `Navigate to an experiment or project worktree and launch your agent.

If the experiment exists in clade's state, it resumes directly.
If not tracked but the branch exists (locally or remotely), it adopts it.

Searches for branches named "exp/<name>" or "feat/<name>". Use --branch to
adopt any existing branch regardless of naming convention.

The SessionStart hook will automatically inject context including:
  - DROPBAG.md from your last session
  - Git status and recent commits
  - Open TODOs
  - Ticket information

Examples:
  clade resume                       # Interactive picker
  clade resume try-redis             # Specific experiment
  clade resume try-redis -r backend  # Adopt branch from specific repo
  clade resume price-formula -r backend --branch feat/price-formula-system
  clade resume try-redis -o cursor   # Resume + open Cursor IDE
  clade resume try-redis -o code     # Resume + open VS Code
  clade resume foo --no-agent        # Skip launching Claude`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runResume,
	ValidArgsFunction: completeResumableNames,
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().StringVarP(&resumeRepoFlag, "repo", "r", "", "Repository for adopting orphaned branches")
	resumeCmd.Flags().StringVarP(&resumeEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	resumeCmd.Flags().StringVarP(&resumeAgentFlag, "agent", "a", "", "Override configured agent")
	resumeCmd.Flags().StringVarP(&resumeBranchFlag, "branch", "b", "", "Exact branch name to adopt (e.g., feat/my-feature)")
	resumeCmd.Flags().BoolVar(&resumeNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	resumeCmd.Flags().BoolVar(&resumeNoEditorFlag, "no-editor", false, "Skip opening the editor")
}

func runResume(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// If no args, show picker (only tracked items)
	if len(args) == 0 {
		return resumeInteractive(cfg, state)
	}

	name := args[0]

	// Build WorktreeOptions from flags
	opts := WorktreeOptions{
		EditorFlag:   resumeEditorFlag,
		AgentFlag:    resumeAgentFlag,
		NoAgentFlag:  resumeNoAgentFlag,
		NoEditorFlag: resumeNoEditorFlag,
	}

	// First, check v2 worktrees
	if repoName, wt := state.FindWorktreeByName(name); wt != nil {
		// Find repo path from registered repos
		repoPath := ""
		for rName, rPath := range cfg.Repos {
			if rName == repoName || filepath.Base(config.ExpandPath(rPath)) == repoName {
				repoPath = config.ExpandPath(rPath)
				break
			}
		}
		return ResumeWorktree(cfg, state, repoName, wt, repoPath, opts)
	}

	// Check v1 experiments (legacy format)
	for _, exp := range state.Experiments {
		if exp.Name == name {
			return resumeTrackedExperiment(cfg, state, exp)
		}
	}

	for _, proj := range state.Projects {
		if proj.Name == name {
			return resumeTrackedProject(cfg, state, proj)
		}
	}

	for _, scratch := range state.Scratches {
		if scratch.Name == name {
			return resumeTrackedScratch(cfg, state, scratch)
		}
	}

	// Not tracked - try to adopt orphaned branch
	return adoptOrphanedBranch(cfg, state, name)
}

func resumeInteractive(cfg *config.Config, state *config.State) error {
	totalItems := len(state.Worktrees) + len(state.Experiments) + len(state.Projects) + len(state.Scratches)
	if totalItems == 0 {
		ui.Info("No worktrees, projects, or scratch folders to resume")
		ui.Detail("Create one with: clade feature <name>")
		ui.Detail("Or for bugs: clade bug <name>")
		ui.Detail("Or for spikes: clade spike <name>")
		ui.Detail("Or for no-git: clade scratch <name>")
		ui.Detail("Or adopt an existing branch: clade resume <branch-name> -r <repo>")
		return nil
	}

	type pickItem struct {
		Name     string
		Path     string
		Type     string // "worktree", "experiment", "project", "scratch"
		RepoName string // for worktrees
		LastUsed time.Time
		Display  string
	}

	var items []pickItem

	// Add v2 worktrees
	for repoName, worktrees := range state.Worktrees {
		for _, wt := range worktrees {
			age := formatAge(wt.LastUsed)
			wtPath := config.WorktreePath(cfg, repoName, wt.Name)
			items = append(items, pickItem{
				Name:     wt.Name,
				Path:     wtPath,
				Type:     "worktree",
				RepoName: repoName,
				LastUsed: wt.LastUsed,
				Display:  fmt.Sprintf("%s %s %s (%s)", wt.Name, ui.Dim("["+wt.Label+"]"), ui.Dim(repoName), ui.Dim(age)),
			})
		}
	}

	// Add v1 experiments (legacy)
	for _, exp := range state.Experiments {
		age := formatAge(exp.LastUsed)
		items = append(items, pickItem{
			Name:     exp.Name,
			Path:     exp.Path,
			Type:     "experiment",
			LastUsed: exp.LastUsed,
			Display:  fmt.Sprintf("%s %s (%s)", exp.Name, ui.Dim("[legacy]"), ui.Dim(age)),
		})
	}

	for _, proj := range state.Projects {
		age := formatAge(proj.LastUsed)
		items = append(items, pickItem{
			Name:     proj.Name,
			Path:     proj.Path,
			Type:     "project",
			LastUsed: proj.LastUsed,
			Display:  fmt.Sprintf("%s %s (%s)", proj.Name, ui.Dim("[project]"), ui.Dim(age)),
		})
	}

	for _, scratch := range state.Scratches {
		age := formatAge(scratch.LastUsed)
		items = append(items, pickItem{
			Name:     scratch.Name,
			Path:     scratch.Path,
			Type:     "scratch",
			LastUsed: scratch.LastUsed,
			Display:  fmt.Sprintf("%s %s (%s)", scratch.Name, ui.Dim("[scratch]"), ui.Dim(age)),
		})
	}

	// Sort by last used (most recent first)
	sort.Slice(items, func(i, j int) bool {
		return items[i].LastUsed.After(items[j].LastUsed)
	})

	var displayItems []string
	for _, item := range items {
		displayItems = append(displayItems, item.Display)
	}

	prompt := promptui.Select{
		Label: "Select to resume",
		Items: displayItems,
		Size:  10,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return err
	}

	opts := WorktreeOptions{
		EditorFlag:   resumeEditorFlag,
		AgentFlag:    resumeAgentFlag,
		NoAgentFlag:  resumeNoAgentFlag,
		NoEditorFlag: resumeNoEditorFlag,
	}

	switch items[idx].Type {
	case "worktree":
		wt := state.GetWorktree(items[idx].RepoName, items[idx].Name)
		if wt != nil {
			// Find repo path
			repoPath := ""
			for rName, rPath := range cfg.Repos {
				if rName == items[idx].RepoName || filepath.Base(config.ExpandPath(rPath)) == items[idx].RepoName {
					repoPath = config.ExpandPath(rPath)
					break
				}
			}
			return ResumeWorktree(cfg, state, items[idx].RepoName, wt, repoPath, opts)
		}
	case "experiment":
		for _, exp := range state.Experiments {
			if exp.Name == items[idx].Name {
				return resumeTrackedExperiment(cfg, state, exp)
			}
		}
	case "project":
		for _, proj := range state.Projects {
			if proj.Name == items[idx].Name {
				return resumeTrackedProject(cfg, state, proj)
			}
		}
	case "scratch":
		for _, scratch := range state.Scratches {
			if scratch.Name == items[idx].Name {
				return resumeTrackedScratch(cfg, state, scratch)
			}
		}
	}

	return nil
}

func resumeTrackedExperiment(cfg *config.Config, state *config.State, exp *config.Experiment) error {
	// Verify path exists
	if _, err := os.Stat(exp.Path); os.IsNotExist(err) {
		ui.Error("Path no longer exists: %s", exp.Path)
		ui.Detail("The worktree may have been removed manually")
		ui.Detail("Run: clade cleanup %s", exp.Name)
		return fmt.Errorf("worktree not found")
	}

	// Check for divergence if remote exists
	git.Fetch(exp.Repo)
	branchInfo := git.CheckBranch(exp.Repo, exp.Branch)
	if branchInfo.Diverged {
		ui.Warn("Branch diverged from origin (%d local, %d remote commits)", branchInfo.LocalAhead, branchInfo.RemoteBehind)
		ui.Detail("Resolve in worktree: git pull --rebase OR git merge")
	} else if branchInfo.RemoteBehind > 0 {
		ui.Info("Remote has %d new commits - consider: git pull", branchInfo.RemoteBehind)
	}

	// Update last used
	exp.LastUsed = time.Now()
	state.Experiments[config.ExperimentKey(exp.Repo, exp.Name)] = exp
	state.Save(cfg)

	ui.Header("Resuming: %s", exp.Name)
	ui.KeyValue("Path", exp.Path)

	// Run on_resume hooks (for legacy experiments too)
	if hooks.HasHooks(hooks.OnResume, exp.Repo) {
		ui.Info("Running on_resume hooks...")
		hookEnv := &hooks.Env{
			Type:     "experiment",
			Name:     exp.Name,
			Path:     exp.Path,
			RepoName: filepath.Base(exp.Repo),
			RepoPath: exp.Repo,
			Branch:   exp.Branch,
			Ticket:   exp.Ticket,
		}
		results := hooks.RunHooks(hooks.OnResume, hookEnv)
		for _, r := range results {
			if r.Error != nil {
				ui.Warn("Hook failed: %s - %v", r.Command, r.Error)
			}
		}
	}

	return launchWorktreeSession(cfg, exp.Repo, exp.Path, WorktreeOptions{
		EditorFlag:   resumeEditorFlag,
		AgentFlag:    resumeAgentFlag,
		NoAgentFlag:  resumeNoAgentFlag,
		NoEditorFlag: resumeNoEditorFlag,
	})
}

func resumeTrackedProject(cfg *config.Config, state *config.State, proj *config.Project) error {
	// Verify path exists
	if _, err := os.Stat(proj.Path); os.IsNotExist(err) {
		ui.Error("Path no longer exists: %s", proj.Path)
		return fmt.Errorf("project directory not found")
	}

	// Check divergence for each repo
	for _, repo := range proj.Repos {
		git.Fetch(repo.Source)
		branchInfo := git.CheckBranch(repo.Source, proj.Branch)
		if branchInfo.Diverged {
			ui.Warn("%s: branch diverged (%d local, %d remote)", repo.Name, branchInfo.LocalAhead, branchInfo.RemoteBehind)
		}
	}

	// Update last used
	proj.LastUsed = time.Now()
	state.Projects[proj.Name] = proj
	state.Save(cfg)

	ui.Header("Resuming: %s", proj.Name)
	ui.KeyValue("Path", proj.Path)

	// Run on_resume hooks for each repo in project
	for _, repo := range proj.Repos {
		if hooks.HasHooks(hooks.OnResume, repo.Source) {
			repoPath := filepath.Join(proj.Path, repo.Name)
			hookEnv := &hooks.Env{
				Type:        "project",
				Name:        proj.Name,
				Path:        repoPath,
				RepoName:    repo.Name,
				RepoPath:    repo.Source,
				Branch:      proj.Branch,
				ProjectName: proj.Name,
			}
			hooks.RunHooks(hooks.OnResume, hookEnv)
		}
	}

	return launchProjectSessionWithAgent(cfg, proj, resumeEditorFlag, resumeAgentFlag, resumeNoAgentFlag, resumeNoEditorFlag)
}

func adoptOrphanedBranch(cfg *config.Config, state *config.State, name string) error {
	// Need to know which repo to check
	repoPath, err := resolveRepo(cfg, resumeRepoFlag)
	if err != nil {
		ui.Error("Cannot adopt branch without knowing which repo")
		ui.Detail("Specify repo: clade resume %s -r <repo>", name)
		return err
	}

	repoName := git.GetRepoName(repoPath)
	git.Fetch(repoPath)

	// Use explicit branch if provided, otherwise search exp/ and feat/ prefixes
	var branch string
	var branchInfo git.BranchInfo

	if resumeBranchFlag != "" {
		branch = resumeBranchFlag
		ui.Info("Checking for branch '%s' in %s...", branch, repoName)
		branchInfo = git.CheckBranch(repoPath, branch)
	} else {
		// Search for exp/ first, then feat/
		expBranch := "exp/" + name
		featBranch := "feat/" + name

		ui.Info("Searching for branches in %s...", repoName)

		expInfo := git.CheckBranch(repoPath, expBranch)
		featInfo := git.CheckBranch(repoPath, featBranch)

		expFound := expInfo.Status != git.BranchNotFound
		featFound := featInfo.Status != git.BranchNotFound

		if expFound && featFound {
			// Both exist - prompt user to choose
			ui.Info("Found both exp/%s and feat/%s", name, name)
			prompt := promptui.Select{
				Label: "Which branch",
				Items: []string{expBranch, featBranch},
			}
			_, branch, err = prompt.Run()
			if err != nil {
				return err
			}
			if branch == expBranch {
				branchInfo = expInfo
			} else {
				branchInfo = featInfo
			}
		} else if expFound {
			branch = expBranch
			branchInfo = expInfo
		} else if featFound {
			branch = featBranch
			branchInfo = featInfo
		} else {
			ui.Error("Branch not found: tried '%s' and '%s'", expBranch, featBranch)
			ui.Detail("Create new experiment: clade exp %s", name)
			ui.Detail("Or specify branch: clade resume %s --branch <branch>", name)
			return fmt.Errorf("branch not found")
		}
	}

	if branchInfo.Status == git.BranchNotFound {
		ui.Error("Branch '%s' not found locally or on remote", branch)
		ui.Detail("Create new experiment: clade exp %s", name)
		return fmt.Errorf("branch not found")
	}

	// Branch exists - adopt it
	expKey := config.ExperimentKey(repoPath, name)
	expPath := filepath.Join(cfg.ExperimentsDir(), expKey)

	// Ensure experiments directory exists
	if err := os.MkdirAll(cfg.ExperimentsDir(), 0755); err != nil {
		return err
	}

	switch branchInfo.Status {
	case git.BranchLocalOnly:
		ui.Info("Adopting local branch '%s'", branch)
		if err := git.CreateWorktreeFromBranch(repoPath, expPath, branch); err != nil {
			return err
		}

	case git.BranchRemoteOnly:
		ui.Info("Tracking remote branch 'origin/%s'", branch)
		if err := git.CreateWorktreeTrackRemote(repoPath, expPath, branch); err != nil {
			return err
		}

	case git.BranchBoth:
		ui.Info("Adopting branch '%s'", branch)
		if err := git.CreateWorktreeFromBranch(repoPath, expPath, branch); err != nil {
			return err
		}
		if branchInfo.Diverged {
			ui.Warn("Branch diverged from origin (%d local, %d remote commits)", branchInfo.LocalAhead, branchInfo.RemoteBehind)
			ui.Detail("Resolve in worktree: git pull --rebase OR git merge")
		}
	}

	// Auto-init if needed
	if cfg.AutoInit {
		InitRepo(expPath)
	}

	// Add to state
	ticket := extractTicket(name)
	exp := &config.Experiment{
		Name:     name,
		Repo:     repoPath,
		Path:     expPath,
		Branch:   branch,
		Ticket:   ticket,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}
	state.AddExperiment(exp)
	state.Save(cfg)

	ui.Success("Adopted experiment '%s'", name)
	ui.KeyValue("Path", expPath)

	return launchSession(cfg, expPath, resumeEditorFlag, resumeNoAgentFlag, resumeNoEditorFlag)
}

func resumeTrackedScratch(cfg *config.Config, state *config.State, scratch *config.Scratch) error {
	// Verify path exists
	if _, err := os.Stat(scratch.Path); os.IsNotExist(err) {
		ui.Error("Path no longer exists: %s", scratch.Path)
		ui.Detail("The scratch folder may have been removed manually")
		ui.Detail("Run: clade cleanup %s", scratch.Name)
		return fmt.Errorf("scratch folder not found")
	}

	// Update last used
	scratch.LastUsed = time.Now()
	state.Scratches[scratch.Name] = scratch
	state.Save(cfg)

	ui.Header("Resuming: %s", scratch.Name)
	ui.KeyValue("Path", scratch.Path)

	// Run on_resume hooks (scratches don't have repo-specific hooks)
	if hooks.HasHooks(hooks.OnResume, "") {
		ui.Info("Running on_resume hooks...")
		hookEnv := &hooks.Env{
			Type: "scratch",
			Name: scratch.Name,
			Path: scratch.Path,
		}
		results := hooks.RunHooks(hooks.OnResume, hookEnv)
		for _, r := range results {
			if r.Error != nil {
				ui.Warn("Hook failed: %s - %v", r.Command, r.Error)
			}
		}
	}

	return launchWorktreeSession(cfg, "", scratch.Path, WorktreeOptions{
		EditorFlag:   resumeEditorFlag,
		AgentFlag:    resumeAgentFlag,
		NoAgentFlag:  resumeNoAgentFlag,
		NoEditorFlag: resumeNoEditorFlag,
	})
}

// completeResumableNames provides shell completion for experiment/project/scratch names
func completeResumableNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, exp := range state.Experiments {
		names = append(names, exp.Name+"\texperiment")
	}
	for _, proj := range state.Projects {
		names = append(names, proj.Name+"\tproject")
	}
	for _, scratch := range state.Scratches {
		names = append(names, scratch.Name+"\tscratch")
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}
