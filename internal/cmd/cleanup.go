package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var cleanupForceFlag bool

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [name]",
	Short: "Remove experiment or project worktrees",
	Long: `Remove an experiment or project and optionally delete the branch.

Examples:
  clade cleanup try-redis           # Clean up experiment
  clade cleanup my-project          # Clean up project
  clade cleanup try-redis --force   # Skip confirmations`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runCleanup,
	ValidArgsFunction: completeCleanupNames,
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
	cleanupCmd.Flags().BoolVarP(&cleanupForceFlag, "force", "f", false, "Skip confirmations")
}

func runCleanup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Check if there's anything to clean up
	if len(state.Experiments) == 0 && len(state.Projects) == 0 && len(state.Scratches) == 0 {
		ui.Info("No experiments, projects, or scratch folders to clean up")
		return nil
	}

	var targetName string
	if len(args) > 0 {
		targetName = args[0]
	} else {
		// Interactive picker combining experiments, projects, and scratches
		type pickItem struct {
			Name string
			Type string // "experiment", "project", or "scratch"
		}
		var items []pickItem
		var displayItems []string

		for _, exp := range state.Experiments {
			items = append(items, pickItem{Name: exp.Name, Type: "experiment"})
			displayItems = append(displayItems, fmt.Sprintf("%s %s", exp.Name, ui.Dim("[exp]")))
		}
		for _, proj := range state.Projects {
			items = append(items, pickItem{Name: proj.Name, Type: "project"})
			displayItems = append(displayItems, fmt.Sprintf("%s %s", proj.Name, ui.Dim("[project]")))
		}
		for _, scratch := range state.Scratches {
			items = append(items, pickItem{Name: scratch.Name, Type: "scratch"})
			displayItems = append(displayItems, fmt.Sprintf("%s %s", scratch.Name, ui.Dim("[scratch]")))
		}

		prompt := promptui.Select{
			Label: "Select to clean up",
			Items: displayItems,
		}
		idx, _, err := prompt.Run()
		if err != nil {
			return err
		}
		targetName = items[idx].Name
	}

	// Try to find as experiment first, then as project, then as scratch
	for key, exp := range state.Experiments {
		if exp.Name == targetName {
			return cleanupExperiment(cfg, state, key, exp)
		}
	}

	for name, proj := range state.Projects {
		if proj.Name == targetName {
			return cleanupProject(cfg, state, name, proj)
		}
	}

	for name, scratch := range state.Scratches {
		if scratch.Name == targetName {
			return cleanupScratch(cfg, state, name, scratch)
		}
	}

	return fmt.Errorf("'%s' not found as experiment, project, or scratch", targetName)
}

func cleanupExperiment(cfg *config.Config, state *config.State, key string, exp *config.Experiment) error {
	ui.Header("Experiment: %s", exp.Name)
	ui.KeyValue("Path", exp.Path)
	ui.KeyValue("Branch", exp.Branch)
	fmt.Println()

	// Check for uncommitted changes
	if hasChanges, _ := git.HasUncommittedChanges(exp.Path); hasChanges {
		ui.Warn("Uncommitted changes detected")

		if !cleanupForceFlag {
			prompt := promptui.Prompt{
				Label:     "Discard changes and continue",
				IsConfirm: true,
			}
			_, err := prompt.Run()
			if err != nil {
				ui.Info("Cleanup cancelled")
				return nil
			}
		}
	}

	// Remove worktree
	ui.Info("Removing worktree...")
	if err := git.RemoveWorktree(exp.Repo, exp.Path); err != nil {
		// Try removing directory manually if worktree removal fails
		if err := os.RemoveAll(exp.Path); err != nil {
			return fmt.Errorf("failed to remove worktree: %w", err)
		}
	}
	ui.Success("Worktree removed")

	// Ask about branch deletion
	deleteBranch := cleanupForceFlag
	if !cleanupForceFlag {
		prompt := promptui.Prompt{
			Label:     fmt.Sprintf("Delete branch %s", exp.Branch),
			IsConfirm: true,
		}
		_, err := prompt.Run()
		deleteBranch = err == nil
	}

	if deleteBranch {
		ui.Info("Deleting branch...")
		if err := git.DeleteBranch(exp.Repo, exp.Branch); err != nil {
			ui.Warn("Failed to delete branch: %v", err)
		} else {
			ui.Success("Branch deleted")
		}
	}

	// Update state
	state.RemoveExperiment(key)
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	ui.Success("Cleaned up experiment '%s'", exp.Name)
	return nil
}

func cleanupProject(cfg *config.Config, state *config.State, name string, proj *config.Project) error {
	ui.Header("Project: %s", proj.Name)
	ui.KeyValue("Path", proj.Path)
	ui.KeyValue("Branch", proj.Branch)

	var repoNames []string
	for _, r := range proj.Repos {
		repoNames = append(repoNames, r.Name)
	}
	ui.KeyValue("Repos", fmt.Sprintf("%v", repoNames))
	fmt.Println()

	// Check for uncommitted changes in any repo
	hasAnyChanges := false
	for _, repo := range proj.Repos {
		repoPath := filepath.Join(proj.Path, repo.Name)
		if hasChanges, _ := git.HasUncommittedChanges(repoPath); hasChanges {
			ui.Warn("Uncommitted changes in %s", repo.Name)
			hasAnyChanges = true
		}
	}

	if hasAnyChanges && !cleanupForceFlag {
		prompt := promptui.Prompt{
			Label:     "Discard all changes and continue",
			IsConfirm: true,
		}
		_, err := prompt.Run()
		if err != nil {
			ui.Info("Cleanup cancelled")
			return nil
		}
	}

	// Remove each worktree
	ui.Info("Removing worktrees...")
	for _, repo := range proj.Repos {
		repoPath := filepath.Join(proj.Path, repo.Name)
		if err := git.RemoveWorktree(repo.Source, repoPath); err != nil {
			// Try removing directory manually
			os.RemoveAll(repoPath)
		}
		ui.Success("Removed %s", repo.Name)
	}

	// Remove project directory
	os.RemoveAll(proj.Path)

	// Ask about branch deletion
	deleteBranch := cleanupForceFlag
	if !cleanupForceFlag {
		prompt := promptui.Prompt{
			Label:     fmt.Sprintf("Delete branch %s from all repos", proj.Branch),
			IsConfirm: true,
		}
		_, err := prompt.Run()
		deleteBranch = err == nil
	}

	if deleteBranch {
		ui.Info("Deleting branches...")
		for _, repo := range proj.Repos {
			if err := git.DeleteBranch(repo.Source, proj.Branch); err != nil {
				ui.Warn("Failed to delete branch in %s: %v", repo.Name, err)
			} else {
				ui.Success("Deleted branch in %s", repo.Name)
			}
		}
	}

	// Update state
	delete(state.Projects, name)
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	ui.Success("Cleaned up project '%s'", proj.Name)
	return nil
}

func cleanupScratch(cfg *config.Config, state *config.State, name string, scratch *config.Scratch) error {
	ui.Header("Scratch: %s", scratch.Name)
	ui.KeyValue("Path", scratch.Path)
	fmt.Println()

	// Check if directory has any files (warn user)
	entries, err := os.ReadDir(scratch.Path)
	if err == nil && len(entries) > 0 {
		// Count non-hidden files
		fileCount := 0
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				fileCount++
			}
		}
		if fileCount > 0 && !cleanupForceFlag {
			ui.Warn("Scratch folder contains %d file(s)", fileCount)
			prompt := promptui.Prompt{
				Label:     "Delete all contents and continue",
				IsConfirm: true,
			}
			_, err := prompt.Run()
			if err != nil {
				ui.Info("Cleanup cancelled")
				return nil
			}
		}
	}

	// Remove directory
	ui.Info("Removing scratch folder...")
	if err := os.RemoveAll(scratch.Path); err != nil {
		return fmt.Errorf("failed to remove scratch folder: %w", err)
	}
	ui.Success("Folder removed")

	// Update state
	state.RemoveScratch(name)
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	ui.Success("Cleaned up scratch '%s'", scratch.Name)
	return nil
}

// completeCleanupNames provides shell completion for experiment/project/scratch names
func completeCleanupNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
