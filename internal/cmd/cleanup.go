package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/hooks"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var cleanupForceFlag bool
var cleanupDryRunFlag bool

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [name]",
	Short: "Remove worktrees, projects, or scratch folders",
	Long: `Remove a worktree, project, or scratch folder and optionally delete the branch.

Examples:
  clade cleanup LEAP-1234             # Clean up worktree
  clade cleanup my-project            # Clean up project
  clade cleanup my-scratch            # Clean up scratch folder
  clade cleanup LEAP-1234 --force     # Skip confirmations
  clade cleanup LEAP-1234 --dry-run   # Preview what would be deleted`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runCleanup,
	ValidArgsFunction: completeCleanupNames,
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
	cleanupCmd.Flags().BoolVarP(&cleanupForceFlag, "force", "f", false, "Skip confirmations")
	cleanupCmd.Flags().BoolVar(&cleanupDryRunFlag, "dry-run", false, "Show what would be deleted without deleting")
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

	// Count total items
	totalWorktrees := 0
	for _, wts := range state.Worktrees {
		totalWorktrees += len(wts)
	}

	// Check if there's anything to clean up
	if totalWorktrees == 0 && len(state.Experiments) == 0 && len(state.Projects) == 0 && len(state.Scratches) == 0 {
		ui.Info("No worktrees, projects, or scratch folders to clean up")
		return nil
	}

	var targetName string
	var targetRepoName string // for v2 worktrees
	if len(args) > 0 {
		targetName = args[0]
	} else {
		// Interactive picker
		type pickItem struct {
			Name     string
			Type     string // "worktree", "experiment", "project", or "scratch"
			RepoName string // for v2 worktrees
		}
		var items []pickItem
		var displayItems []string

		// Add v2 worktrees
		for repoName, worktrees := range state.Worktrees {
			for _, wt := range worktrees {
				items = append(items, pickItem{Name: wt.Name, Type: "worktree", RepoName: repoName})
				displayItems = append(displayItems, fmt.Sprintf("%s %s %s", wt.Name, ui.Dim("["+wt.Label+"]"), ui.Dim(repoName)))
			}
		}

		// Add v1 experiments (legacy)
		for _, exp := range state.Experiments {
			items = append(items, pickItem{Name: exp.Name, Type: "experiment"})
			displayItems = append(displayItems, fmt.Sprintf("%s %s", exp.Name, ui.Dim("[legacy]")))
		}
		for _, proj := range state.Projects {
			items = append(items, pickItem{Name: proj.Name, Type: "project"})
			displayItems = append(displayItems, fmt.Sprintf("%s %s", proj.Name, ui.Dim("[project]")))
		}
		for _, scratch := range state.Scratches {
			items = append(items, pickItem{Name: scratch.Name, Type: "scratch"})
			displayItems = append(displayItems, fmt.Sprintf("%s %s", scratch.Name, ui.Dim("[scratch]")))
		}

		// Sort by name
		sort.Slice(displayItems, func(i, j int) bool {
			return displayItems[i] < displayItems[j]
		})

		prompt := promptui.Select{
			Label: "Select to clean up",
			Items: displayItems,
		}
		idx, _, err := prompt.Run()
		if err != nil {
			return err
		}
		targetName = items[idx].Name
		targetRepoName = items[idx].RepoName
	}

	// Try to find as v2 worktree first
	if repoName, wt := state.FindWorktreeByName(targetName); wt != nil {
		// Use the found repo name if not already set
		if targetRepoName == "" {
			targetRepoName = repoName
		}
		return cleanupWorktree(cfg, state, targetRepoName, wt)
	}

	// Try v1 experiments (legacy)
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

	return fmt.Errorf("'%s' not found as worktree, project, or scratch", targetName)
}

func cleanupWorktree(cfg *config.Config, state *config.State, repoName string, wt *config.Worktree) error {
	wtPath := config.GetWorktreePath(cfg, repoName, wt)

	// Dry-run mode: just show what would be deleted
	if cleanupDryRunFlag {
		ui.Info("[DRY RUN] Would clean up worktree: %s", wt.Name)
		ui.Detail("  Path: %s", wtPath)
		ui.Detail("  Branch: %s", wt.Branch)
		ui.Detail("  Label: %s", wt.Label)
		// Check for dropbags
		dropbagsDir := filepath.Join(wtPath, ".clade", "dropbags")
		if entries, err := os.ReadDir(dropbagsDir); err == nil {
			dropbagCount := 0
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "DROPBAG-") {
					dropbagCount++
				}
			}
			if dropbagCount > 0 {
				ui.Detail("  Would archive %d dropbag(s) to ~/.config/clade/archive/", dropbagCount)
			}
		}
		ui.Detail("  Would prompt for branch deletion")
		return nil
	}

	// Find repo path from registered repos
	repoPath := ""
	for rName, rPath := range cfg.Repos {
		if rName == repoName || filepath.Base(config.ExpandPath(rPath)) == repoName {
			repoPath = config.ExpandPath(rPath)
			break
		}
	}

	ui.Header("Worktree: %s", wt.Name)
	ui.KeyValue("Path", wtPath)
	ui.KeyValue("Branch", wt.Branch)
	ui.KeyValue("Label", wt.Label)
	fmt.Println()

	// Run on_remove hooks
	if hooks.HasHooks(hooks.OnRemove, repoPath) {
		ui.Info("Running on_remove hooks...")
		hookEnv := &hooks.Env{
			Type:     wt.Label,
			Name:     wt.Name,
			Path:     wtPath,
			RepoName: repoName,
			RepoPath: repoPath,
			Branch:   wt.Branch,
			Ticket:   wt.Ticket,
		}
		results := hooks.RunHooks(hooks.OnRemove, hookEnv)
		for _, r := range results {
			if r.Error != nil {
				ui.Warn("Hook failed: %s - %v", r.Command, r.Error)
			}
		}
	}

	// Check for uncommitted changes
	if hasChanges, _ := git.HasUncommittedChanges(wtPath); hasChanges {
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

	// Archive dropbags before removing worktree
	if archivePath, err := archiveDropbags(wtPath, repoName, wt.Name); err != nil {
		ui.Warn("Failed to archive session context: %v", err)
	} else if archivePath != "" {
		ui.Success("Archived session context to %s", archivePath)
	}

	// Remove worktree
	ui.Info("Removing worktree...")
	if repoPath != "" {
		if err := git.RemoveWorktree(repoPath, wtPath); err != nil {
			// Try removing directory manually if worktree removal fails
			if err := os.RemoveAll(wtPath); err != nil {
				return fmt.Errorf("failed to remove worktree: %w", err)
			}
		}
	} else {
		// No repo path found, just remove directory
		if err := os.RemoveAll(wtPath); err != nil {
			return fmt.Errorf("failed to remove worktree: %w", err)
		}
	}
	ui.Success("Worktree removed")

	// Ask about branch deletion
	deleteBranch := cleanupForceFlag
	if !cleanupForceFlag && repoPath != "" {
		prompt := promptui.Prompt{
			Label:     fmt.Sprintf("Delete branch %s", wt.Branch),
			IsConfirm: true,
		}
		_, err := prompt.Run()
		deleteBranch = err == nil
	}

	if deleteBranch && repoPath != "" {
		ui.Info("Deleting branch...")
		if err := git.DeleteBranch(repoPath, wt.Branch); err != nil {
			ui.Warn("Failed to delete branch: %v", err)
		} else {
			ui.Success("Branch deleted")
		}
	}

	// Update state
	state.RemoveWorktree(repoName, wt.Name)
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	// Clean up empty repo directory
	repoDir := filepath.Join(cfg.ReposDir(), repoName)
	if entries, err := os.ReadDir(repoDir); err == nil && len(entries) == 0 {
		os.Remove(repoDir)
	}

	ui.Success("Cleaned up worktree '%s'", wt.Name)
	return nil
}

func cleanupExperiment(cfg *config.Config, state *config.State, key string, exp *config.Experiment) error {
	ui.Header("Experiment (legacy): %s", exp.Name)
	ui.KeyValue("Path", exp.Path)
	ui.KeyValue("Branch", exp.Branch)
	fmt.Println()

	// Dry-run mode
	if cleanupDryRunFlag {
		ui.Info("[DRY RUN] Would clean up experiment: %s", exp.Name)
		ui.Detail("  Path: %s", exp.Path)
		ui.Detail("  Branch: %s", exp.Branch)
		// Check for dropbags
		dropbagsDir := filepath.Join(exp.Path, ".clade", "dropbags")
		if entries, err := os.ReadDir(dropbagsDir); err == nil {
			dropbagCount := 0
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "DROPBAG-") {
					dropbagCount++
				}
			}
			if dropbagCount > 0 {
				ui.Detail("  Would archive %d dropbag(s) to ~/.config/clade/archive/", dropbagCount)
			}
		}
		ui.Detail("  Would prompt for branch deletion")
		return nil
	}

	// Run on_remove hooks
	if hooks.HasHooks(hooks.OnRemove, exp.Repo) {
		ui.Info("Running on_remove hooks...")
		hookEnv := &hooks.Env{
			Type:     "experiment",
			Name:     exp.Name,
			Path:     exp.Path,
			RepoName: filepath.Base(exp.Repo),
			RepoPath: exp.Repo,
			Branch:   exp.Branch,
			Ticket:   exp.Ticket,
		}
		results := hooks.RunHooks(hooks.OnRemove, hookEnv)
		for _, r := range results {
			if r.Error != nil {
				ui.Warn("Hook failed: %s - %v", r.Command, r.Error)
			}
		}
	}

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

	// Archive dropbags before removing worktree
	if archivePath, err := archiveDropbags(exp.Path, filepath.Base(exp.Repo), exp.Name); err != nil {
		ui.Warn("Failed to archive session context: %v", err)
	} else if archivePath != "" {
		ui.Success("Archived session context to %s", archivePath)
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

	// Dry-run mode
	if cleanupDryRunFlag {
		ui.Info("[DRY RUN] Would clean up project: %s", proj.Name)
		ui.Detail("  Path: %s", proj.Path)
		ui.Detail("  Branch: %s", proj.Branch)
		ui.Detail("  Repos: %d worktrees", len(proj.Repos))
		for _, repo := range proj.Repos {
			ui.Detail("    - %s (from %s)", repo.Name, filepath.Base(repo.Source))
		}
		ui.Detail("  Would prompt for branch deletion")
		return nil
	}

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

	// Dry-run mode
	if cleanupDryRunFlag {
		ui.Info("[DRY RUN] Would clean up scratch folder: %s", scratch.Name)
		ui.Detail("  Path: %s", scratch.Path)
		// Count files
		if entries, err := os.ReadDir(scratch.Path); err == nil {
			fileCount := 0
			for _, e := range entries {
				if !strings.HasPrefix(e.Name(), ".") {
					fileCount++
				}
			}
			if fileCount > 0 {
				ui.Detail("  Contains %d file(s)", fileCount)
			}
		}
		return nil
	}

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

// completeCleanupNames provides shell completion for worktree/project/scratch names
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
	// v2 worktrees
	for repoName, worktrees := range state.Worktrees {
		for _, wt := range worktrees {
			names = append(names, wt.Name+"\t"+wt.Label+" ("+repoName+")")
		}
	}
	// v1 experiments (legacy)
	for _, exp := range state.Experiments {
		names = append(names, exp.Name+"\tlegacy")
	}
	for _, proj := range state.Projects {
		names = append(names, proj.Name+"\tproject")
	}
	for _, scratch := range state.Scratches {
		names = append(names, scratch.Name+"\tscratch")
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// archiveDropbags copies dropbag files and metadata to the archive directory
// before a worktree is removed. This preserves session history for future reference.
// Archive location: ~/.config/clade/archive/{repo}/{name}-{timestamp}/
func archiveDropbags(wtPath, repoName, wtName string) (string, error) {
	dropbagsDir := filepath.Join(wtPath, ".clade", "dropbags")

	// Check if dropbags directory exists
	if _, err := os.Stat(dropbagsDir); os.IsNotExist(err) {
		return "", nil // No dropbags to archive
	}

	// List dropbag files
	entries, err := os.ReadDir(dropbagsDir)
	if err != nil {
		return "", err
	}

	// Filter for DROPBAG-*.md files
	var dropbagFiles []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "DROPBAG-") && strings.HasSuffix(e.Name(), ".md") {
			dropbagFiles = append(dropbagFiles, e)
		}
	}

	if len(dropbagFiles) == 0 {
		return "", nil // No dropbags to archive
	}

	// Create archive directory: ~/.config/clade/archive/{repo}/{name}-{timestamp}/
	timestamp := time.Now().Format("20060102-1504")
	archiveDir := filepath.Join(config.ArchiveDir(), repoName, fmt.Sprintf("%s-%s", wtName, timestamp))

	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Copy dropbag files
	for _, e := range dropbagFiles {
		src := filepath.Join(dropbagsDir, e.Name())
		dst := filepath.Join(archiveDir, e.Name())
		if err := copyFile(src, dst); err != nil {
			ui.Warn("Failed to archive %s: %v", e.Name(), err)
		}
	}

	// Copy .clade.json metadata if it exists
	cladeJSON := filepath.Join(wtPath, ".clade.json")
	if _, err := os.Stat(cladeJSON); err == nil {
		dst := filepath.Join(archiveDir, ".clade.json")
		if err := copyFile(cladeJSON, dst); err != nil {
			ui.Warn("Failed to archive .clade.json: %v", err)
		}
	}

	return archiveDir, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}
