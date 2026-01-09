package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/agent"
	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/files"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var projectAgentFlag string

var projectCmd = &cobra.Command{
	Use:   "project [name]",
	Short: "Create multi-repo workspace with unified branch",
	Long: `Create a project workspace containing worktrees from multiple repositories.

All repos in the project share the same branch name, making it easy to
coordinate changes across repositories for a single feature.

Examples:
  clade project                     # Interactive setup
  clade project api-integration     # Named project with interactive repo selection

Creates:
  ~/clade/projects/{name}/
    ├── backend/      # Worktree from repo 1
    ├── frontend/     # Worktree from repo 2
    └── shared/       # Worktree from repo 3`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProject,
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.Flags().StringVarP(&projectAgentFlag, "agent", "a", "", "Agent to launch (overrides config)")
}

type projectRepo struct {
	SourcePath string // Original repo path
	FolderName string // Name in project directory
}

func runProject(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get project name
	var projectName string
	if len(args) > 0 {
		projectName = args[0]
	} else {
		prompt := promptui.Prompt{
			Label: "Project name",
		}
		projectName, err = prompt.Run()
		if err != nil {
			return err
		}
	}

	if !isValidExpName(projectName) {
		return fmt.Errorf("invalid project name: use alphanumeric, hyphens, underscores only")
	}

	// Check if project already exists
	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if existing, ok := state.Projects[projectName]; ok {
		ui.Warn("Project '%s' already exists", projectName)
		ui.KeyValue("Path", existing.Path)

		prompt := promptui.Prompt{
			Label:     "Resume existing project",
			IsConfirm: true,
		}
		_, err := prompt.Run()
		if err == nil {
			return launchProjectAgent(cfg, existing, projectAgentFlag)
		}
		return nil
	}

	// Get branch name
	prompt := promptui.Prompt{
		Label:   "Branch name",
		Default: "feat/" + projectName,
	}
	branchName, err := prompt.Run()
	if err != nil {
		return err
	}

	// Collect repos
	ui.Header("Add repositories")
	ui.Detail("Enter repo path or registered name (blank when done)")
	fmt.Println()

	var repos []projectRepo

	for {
		// Show registered repos as hint
		if len(cfg.Repos) > 0 && len(repos) == 0 {
			ui.Detail("Registered repos: %s", strings.Join(getRepoNames(cfg), ", "))
		}

		prompt := promptui.Prompt{
			Label: "Repo",
		}
		repoInput, err := prompt.Run()
		if err != nil || repoInput == "" {
			break
		}

		// Resolve repo path
		repoPath, err := resolveRepoPath(cfg, repoInput)
		if err != nil {
			ui.Error("%v", err)
			continue
		}

		// Check not already added
		for _, r := range repos {
			if r.SourcePath == repoPath {
				ui.Warn("Repo already added")
				continue
			}
		}

		// Get folder name
		defaultName := filepath.Base(repoPath)
		folderPrompt := promptui.Prompt{
			Label:   "  Folder name",
			Default: defaultName,
		}
		folderName, err := folderPrompt.Run()
		if err != nil {
			continue
		}

		repos = append(repos, projectRepo{
			SourcePath: repoPath,
			FolderName: folderName,
		})

		ui.Success("Added %s -> %s", filepath.Base(repoPath), folderName)
	}

	if len(repos) == 0 {
		return fmt.Errorf("no repositories added")
	}

	if len(repos) < 2 {
		ui.Warn("Only one repo added. Consider using 'clade exp' for single-repo work.")
		prompt := promptui.Prompt{
			Label:     "Continue anyway",
			IsConfirm: true,
		}
		if _, err := prompt.Run(); err != nil {
			return nil
		}
	}

	// Preflight check: check branch status for all repos before creating anything
	ui.Info("Checking branches...")
	fmt.Println()

	var repoPaths []string
	for _, r := range repos {
		repoPaths = append(repoPaths, r.SourcePath)
	}

	branchResults := git.PreflightCheck(repoPaths, branchName)
	hasWarnings := false

	for _, repo := range repos {
		info := branchResults[repo.SourcePath]
		repoName := repo.FolderName

		switch info.Status {
		case git.BranchNotFound:
			ui.Success("  %s: will create new branch", repoName)
		case git.BranchLocalOnly:
			ui.Warn("  %s: local branch exists (will use existing)", repoName)
			hasWarnings = true
		case git.BranchRemoteOnly:
			ui.Info("  %s: will track remote branch", repoName)
		case git.BranchBoth:
			if info.Diverged {
				ui.Warn("  %s: exists, diverged (%d local, %d remote commits)", repoName, info.LocalAhead, info.RemoteBehind)
				hasWarnings = true
			} else if info.RemoteBehind > 0 {
				ui.Warn("  %s: exists, %d commits behind remote", repoName, info.RemoteBehind)
				hasWarnings = true
			} else if info.LocalAhead > 0 {
				ui.Info("  %s: exists, %d commits ahead of remote", repoName, info.LocalAhead)
			} else {
				ui.Info("  %s: exists, in sync with remote", repoName)
			}
		}
	}

	fmt.Println()

	if hasWarnings {
		prompt := promptui.Prompt{
			Label:     "Warnings detected. Proceed anyway",
			IsConfirm: true,
		}
		if _, err := prompt.Run(); err != nil {
			ui.Info("Aborted.")
			return nil
		}
	}

	// Create project
	projectPath := filepath.Join(cfg.ProjectsDir(), projectName)

	ui.Header("Creating project: %s", projectName)
	ui.KeyValue("Path", projectPath)
	ui.KeyValue("Branch", branchName)
	fmt.Println()

	// Create project directory
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create worktrees for each repo
	var createdRepos []config.ProjectRepo
	for _, repo := range repos {
		worktreePath := filepath.Join(projectPath, repo.FolderName)
		info := branchResults[repo.SourcePath]

		ui.Info("Creating %s...", repo.FolderName)

		var wtErr error
		switch info.Status {
		case git.BranchNotFound:
			wtErr = git.CreateWorktreeNew(repo.SourcePath, worktreePath, branchName)
		case git.BranchLocalOnly, git.BranchBoth:
			wtErr = git.CreateWorktreeFromBranch(repo.SourcePath, worktreePath, branchName)
		case git.BranchRemoteOnly:
			wtErr = git.CreateWorktreeTrackRemote(repo.SourcePath, worktreePath, branchName)
		}

		if wtErr != nil {
			ui.Error("Failed to create worktree for %s: %v", repo.FolderName, wtErr)
			// Clean up on failure
			cleanupPartialProject(projectPath, createdRepos)
			return wtErr
		}

		// Auto-init .claude/ if configured
		if cfg.AutoInit {
			if err := InitRepo(worktreePath); err != nil {
				ui.Warn("Failed to init %s: %v", repo.FolderName, err)
			}
		}

		// Copy gitignored files (.env, .npmrc, etc.)
		if err := copyGitignoredFilesForProject(cfg, repo.SourcePath, worktreePath); err != nil {
			ui.Warn("Failed to copy some files for %s: %v", repo.FolderName, err)
		}

		createdRepos = append(createdRepos, config.ProjectRepo{
			Name:   repo.FolderName,
			Source: repo.SourcePath,
		})

		ui.Success("Created %s", repo.FolderName)
	}

	// Create .clade-project.json
	projectMeta := map[string]interface{}{
		"type":    "project",
		"name":    projectName,
		"branch":  branchName,
		"repos":   createdRepos,
		"created": time.Now().Format(time.RFC3339),
	}

	metaPath := filepath.Join(projectPath, ".clade-project.json")
	if err := writeProjectJSON(metaPath, projectMeta); err != nil {
		ui.Warn("Failed to write .clade-project.json: %v", err)
	}

	// Update state
	project := &config.Project{
		Name:     projectName,
		Path:     projectPath,
		Branch:   branchName,
		Repos:    createdRepos,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}
	state.Projects[projectName] = project
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	fmt.Println()
	ui.Success("Project created!")

	// Launch agent
	return launchProjectAgent(cfg, project, projectAgentFlag)
}

func getRepoNames(cfg *config.Config) []string {
	var names []string
	for name := range cfg.Repos {
		names = append(names, name)
	}
	return names
}

func resolveRepoPath(cfg *config.Config, input string) (string, error) {
	// Check if it's a registered repo name
	if path, ok := cfg.Repos[input]; ok {
		return config.ExpandPath(path), nil
	}

	// Try as path
	expanded := config.ExpandPath(input)
	absPath, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("invalid path: %s", input)
	}

	if !git.IsGitRepo(absPath) {
		return "", fmt.Errorf("not a git repository: %s", input)
	}

	return git.GetRepoRoot(absPath)
}

func launchProjectAgent(cfg *config.Config, project *config.Project, agentOverride string) error {
	agentCmd := cfg.Agent
	if agentOverride != "" {
		agentCmd = agentOverride
	}

	ui.Info("Launching %s...", agentCmd)
	fmt.Println()

	// For multi-repo, we launch in the first repo and add others
	if len(project.Repos) == 0 {
		return fmt.Errorf("project has no repos")
	}

	primaryDir := filepath.Join(project.Path, project.Repos[0].Name)

	// Build add-dir list for other repos
	var addDirs []string
	for i := 1; i < len(project.Repos); i++ {
		addDirs = append(addDirs, filepath.Join(project.Path, project.Repos[i].Name))
	}

	ag := agent.NewAgent(agentCmd)
	opts := agent.LaunchOptions{
		AddDirs: addDirs,
		Flags:   cfg.AgentFlags,
	}

	return ag.Launch(primaryDir, opts)
}

func cleanupPartialProject(projectPath string, created []config.ProjectRepo) {
	ui.Warn("Cleaning up partial project...")
	for _, repo := range created {
		worktreePath := filepath.Join(projectPath, repo.Name)
		os.RemoveAll(worktreePath)
	}
	os.Remove(projectPath)
}

func writeProjectJSON(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// copyGitignoredFilesForProject handles copying of gitignored files for each repo in a project
// Uses saved preferences if available, otherwise prompts interactively
func copyGitignoredFilesForProject(cfg *config.Config, srcRepo, dstPath string) error {
	// Check if we have saved preferences for this repo
	savedFiles := cfg.GetRepoCopyFiles(srcRepo)

	if savedFiles != nil {
		// Use saved preferences silently
		if len(savedFiles) > 0 {
			if err := files.CopyFiles(srcRepo, dstPath, savedFiles); err != nil {
				return err
			}
			for _, f := range savedFiles {
				ui.Detail("    Copied %s", f)
			}
		}
		return nil
	}

	// No saved preferences - detect and prompt
	detected := files.FindGitignored(srcRepo)
	if len(detected) == 0 {
		return nil
	}

	repoName := filepath.Base(srcRepo)
	fmt.Println()
	ui.Info("Found gitignored files in %s:", repoName)
	for _, f := range detected {
		ui.Detail("  %s", f)
	}

	// Interactive selection
	var selected []string
	for _, file := range detected {
		prompt := promptui.Prompt{
			Label:     fmt.Sprintf("Copy %s", file),
			IsConfirm: true,
			Default:   "y",
		}
		_, err := prompt.Run()
		if err == nil {
			selected = append(selected, file)
		}
	}

	// Save preference for future
	cfg.SetRepoCopyFiles(srcRepo, selected)
	if err := cfg.Save(); err != nil {
		ui.Warn("Failed to save file preferences: %v", err)
	}

	// Copy selected files
	if len(selected) > 0 {
		if err := files.CopyFiles(srcRepo, dstPath, selected); err != nil {
			return err
		}
	}

	return nil
}
