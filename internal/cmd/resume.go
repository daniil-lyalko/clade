package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	resumeRepoFlag   string
	resumeAgentFlag  string
)

var resumeCmd = &cobra.Command{
	Use:   "resume [name]",
	Short: "Resume an experiment or project",
	Long: `Navigate to an experiment or project worktree and launch your agent.

If the experiment exists in clade's state, it resumes directly.
If not tracked but the branch exists (locally or remotely), it adopts it.

The SessionStart hook will automatically inject context including:
  - DROPBAG.md from your last session
  - Git status and recent commits
  - Open TODOs
  - Ticket information

Examples:
  clade resume                  # Interactive picker
  clade resume try-redis        # Specific experiment
  clade resume try-redis -r backend  # Adopt branch from specific repo`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runResume,
	ValidArgsFunction: completeResumableNames,
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().StringVarP(&resumeRepoFlag, "repo", "r", "", "Repository for adopting orphaned branches")
	resumeCmd.Flags().StringVarP(&resumeAgentFlag, "agent", "a", "", "Agent to launch (overrides config)")
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

	// First, check if it's already tracked
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
	if len(state.Experiments) == 0 && len(state.Projects) == 0 && len(state.Scratches) == 0 {
		ui.Info("No experiments, projects, or scratch folders to resume")
		ui.Detail("Create one with: clade exp <name>")
		ui.Detail("Or for no-git: clade scratch <name>")
		ui.Detail("Or adopt an existing branch: clade resume <branch-name> -r <repo>")
		return nil
	}

	type pickItem struct {
		Name     string
		Path     string
		Type     string
		LastUsed time.Time
		Display  string
	}

	var items []pickItem

	for _, exp := range state.Experiments {
		age := formatAge(exp.LastUsed)
		items = append(items, pickItem{
			Name:     exp.Name,
			Path:     exp.Path,
			Type:     "experiment",
			LastUsed: exp.LastUsed,
			Display:  fmt.Sprintf("%s %s (%s)", exp.Name, ui.Dim("[exp]"), ui.Dim(age)),
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

	// Sort by last used
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].LastUsed.After(items[i].LastUsed) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

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

	switch items[idx].Type {
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

	return launchAgent(cfg, exp.Path, resumeAgentFlag)
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

	return launchProjectAgent(cfg, proj, resumeAgentFlag)
}

func adoptOrphanedBranch(cfg *config.Config, state *config.State, name string) error {
	// Need to know which repo to check
	repoPath, err := resolveRepo(cfg, resumeRepoFlag)
	if err != nil {
		ui.Error("Cannot adopt branch without knowing which repo")
		ui.Detail("Specify repo: clade resume %s -r <repo>", name)
		return err
	}

	branch := "exp/" + name
	repoName := git.GetRepoName(repoPath)

	ui.Info("Checking for branch '%s' in %s...", branch, repoName)
	git.Fetch(repoPath)

	branchInfo := git.CheckBranch(repoPath, branch)

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

	return launchAgent(cfg, expPath, resumeAgentFlag)
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

	return launchAgent(cfg, scratch.Path, resumeAgentFlag)
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
