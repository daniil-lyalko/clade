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

var (
	projectEditorFlag    string
	projectNoAgentFlag   bool
	projectNoEditorFlag  bool
	projectAddEditorFlag string
	projectAddNoAgentFlag  bool
	projectAddNoEditorFlag bool
)

var projectCmd = &cobra.Command{
	Use:   "project [name]",
	Short: "Create multi-repo workspace with unified branch",
	Long: `Create a project workspace containing worktrees from multiple repositories.

All repos in the project share the same branch name, making it easy to
coordinate changes across repositories for a single feature.

Examples:
  clade project                     # Interactive setup
  clade project api-integration     # Named project with interactive repo selection
  clade project foo -o cursor       # Open Cursor IDE
  clade project foo --no-agent      # Skip launching Claude

Creates:
  ~/clade/projects/{name}/
    ├── backend/      # Worktree from repo 1
    ├── frontend/     # Worktree from repo 2
    └── shared/       # Worktree from repo 3`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProject,
}

var projectAddCmd = &cobra.Command{
	Use:   "add [project] [repo]",
	Short: "Add a repository to an existing project",
	Long: `Add a new repository to an existing project.

The new repo will use the same branch name as the existing project.

Examples:
  clade project add                           # Interactive: pick project and repo
  clade project add api-integration           # Pick repo from registered repos
  clade project add api-integration backend   # Fully specified`,
	Args: cobra.MaximumNArgs(2),
	RunE: runProjectAdd,
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectAddCmd)
	projectCmd.Flags().StringVarP(&projectEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	projectCmd.Flags().StringVarP(&projectEditorFlag, "editor", "e", "", "Alias for --open")
	projectCmd.Flags().BoolVar(&projectNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	projectCmd.Flags().BoolVar(&projectNoEditorFlag, "no-editor", false, "Skip opening the editor")
	projectAddCmd.Flags().StringVarP(&projectAddEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	projectAddCmd.Flags().StringVarP(&projectAddEditorFlag, "editor", "e", "", "Alias for --open")
	projectAddCmd.Flags().BoolVar(&projectAddNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	projectAddCmd.Flags().BoolVar(&projectAddNoEditorFlag, "no-editor", false, "Skip opening the editor")
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
			return launchProjectSession(cfg, existing, projectEditorFlag, projectNoAgentFlag, projectNoEditorFlag)
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

	// Launch editor and/or agent
	return launchProjectSession(cfg, project, projectEditorFlag, projectNoAgentFlag, projectNoEditorFlag)
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

// launchProjectSession opens editor and/or launches agent for a project
func launchProjectSession(cfg *config.Config, project *config.Project, editorOverride string, noAgent bool, noEditor bool) error {
	if len(project.Repos) == 0 {
		return fmt.Errorf("project has no repos")
	}

	primaryDir := filepath.Join(project.Path, project.Repos[0].Name)

	// Determine editor to use
	editor := cfg.Editor
	if editorOverride != "" {
		editor = editorOverride
	}

	// Open editor first (if configured and not disabled)
	if !noEditor && editor != "" {
		opts := agent.EditorOptions{
			TmuxSplitDirection: cfg.TmuxSplitDirection,
		}
		// Open editor at project root to see all repos
		if err := agent.OpenEditor(project.Path, editor, opts); err != nil {
			ui.Warn("Could not open editor: %s", err)
		} else {
			ui.Info("Opened %s", editor)
		}
	}

	// Launch agent (if configured and not disabled)
	if !noAgent && cfg.Agent != "" {
		ui.Info("Launching %s...", cfg.Agent)
		fmt.Println()

		// Build add-dir list for other repos
		var addDirs []string
		for i := 1; i < len(project.Repos); i++ {
			addDirs = append(addDirs, filepath.Join(project.Path, project.Repos[i].Name))
		}

		ag := agent.NewAgent(cfg.Agent)
		opts := agent.LaunchOptions{
			AddDirs: addDirs,
			Flags:   cfg.AgentFlags,
		}

		return ag.Launch(primaryDir, opts)
	}

	return nil
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

func runProjectAdd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Check if there are any projects
	if len(state.Projects) == 0 {
		return fmt.Errorf("no projects found. Create one first with: clade project <name>")
	}

	// Get project name
	var projectName string
	if len(args) >= 1 {
		projectName = args[0]
	} else {
		// Interactive project picker
		projectName, err = pickProject(state)
		if err != nil {
			return err
		}
	}

	// Find the project
	project, ok := state.Projects[projectName]
	if !ok {
		return fmt.Errorf("project '%s' not found", projectName)
	}

	// Check if there are registered repos
	if len(cfg.Repos) == 0 {
		return fmt.Errorf("no registered repos. Add some with: clade repo add <path>")
	}

	// Get repo to add
	var repoName string
	var repoPath string
	if len(args) >= 2 {
		repoName = args[1]
		// Resolve repo path
		repoPath, err = resolveRepoPath(cfg, repoName)
		if err != nil {
			return err
		}
	} else {
		// Interactive repo picker - filter out repos already in project
		repoName, repoPath, err = pickRepoToAdd(cfg, project)
		if err != nil {
			return err
		}
	}

	// Check if repo is already in project
	for _, r := range project.Repos {
		if config.ExpandPath(r.Source) == repoPath {
			return fmt.Errorf("repo '%s' is already in project '%s'", filepath.Base(repoPath), projectName)
		}
	}

	// Get folder name for the new repo
	defaultName := filepath.Base(repoPath)
	folderPrompt := promptui.Prompt{
		Label:   "Folder name",
		Default: defaultName,
	}
	folderName, err := folderPrompt.Run()
	if err != nil {
		return err
	}

	// Check folder name doesn't conflict
	for _, r := range project.Repos {
		if r.Name == folderName {
			return fmt.Errorf("folder name '%s' already exists in project", folderName)
		}
	}

	// Preflight check for branch
	ui.Info("Checking branch '%s'...", project.Branch)
	branchResults := git.PreflightCheck([]string{repoPath}, project.Branch)
	info := branchResults[repoPath]

	switch info.Status {
	case git.BranchNotFound:
		ui.Success("Will create new branch")
	case git.BranchLocalOnly:
		ui.Warn("Local branch exists (will use existing)")
	case git.BranchRemoteOnly:
		ui.Info("Will track remote branch")
	case git.BranchBoth:
		if info.Diverged {
			ui.Warn("Branch exists, diverged (%d local, %d remote commits)", info.LocalAhead, info.RemoteBehind)
		} else {
			ui.Info("Branch exists and in sync")
		}
	}
	fmt.Println()

	// Create worktree
	worktreePath := filepath.Join(project.Path, folderName)

	ui.Header("Adding to project: %s", projectName)
	ui.KeyValue("Repo", filepath.Base(repoPath))
	ui.KeyValue("Folder", folderName)
	ui.KeyValue("Branch", project.Branch)
	fmt.Println()

	ui.Info("Creating worktree...")

	var wtErr error
	switch info.Status {
	case git.BranchNotFound:
		wtErr = git.CreateWorktreeNew(repoPath, worktreePath, project.Branch)
	case git.BranchLocalOnly, git.BranchBoth:
		wtErr = git.CreateWorktreeFromBranch(repoPath, worktreePath, project.Branch)
	case git.BranchRemoteOnly:
		wtErr = git.CreateWorktreeTrackRemote(repoPath, worktreePath, project.Branch)
	}

	if wtErr != nil {
		return fmt.Errorf("failed to create worktree: %w", wtErr)
	}

	// Auto-init .claude/ if configured
	if cfg.AutoInit {
		if err := InitRepo(worktreePath); err != nil {
			ui.Warn("Failed to init: %v", err)
		}
	}

	// Copy gitignored files
	if err := copyGitignoredFilesForProject(cfg, repoPath, worktreePath); err != nil {
		ui.Warn("Failed to copy some files: %v", err)
	}

	// Update project in state
	newRepo := config.ProjectRepo{
		Name:   folderName,
		Source: repoPath,
	}
	project.Repos = append(project.Repos, newRepo)
	project.LastUsed = time.Now()

	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	// Update .clade-project.json
	projectMeta := map[string]interface{}{
		"type":    "project",
		"name":    project.Name,
		"branch":  project.Branch,
		"repos":   project.Repos,
		"created": project.Created.Format(time.RFC3339),
	}

	metaPath := filepath.Join(project.Path, ".clade-project.json")
	if err := writeProjectJSON(metaPath, projectMeta); err != nil {
		ui.Warn("Failed to update .clade-project.json: %v", err)
	}

	ui.Success("Added %s to project!", folderName)
	fmt.Println()

	// Ask if user wants to launch agent
	prompt := promptui.Prompt{
		Label:     "Launch agent",
		IsConfirm: true,
		Default:   "y",
	}
	if _, err := prompt.Run(); err == nil {
		return launchProjectSession(cfg, project, projectAddEditorFlag, projectAddNoAgentFlag, projectAddNoEditorFlag)
	}

	return nil
}

func pickProject(state *config.State) (string, error) {
	var projectNames []string
	for name := range state.Projects {
		projectNames = append(projectNames, name)
	}

	if len(projectNames) == 1 {
		return projectNames[0], nil
	}

	prompt := promptui.Select{
		Label: "Select project",
		Items: projectNames,
	}

	_, result, err := prompt.Run()
	return result, err
}

func pickRepoToAdd(cfg *config.Config, project *config.Project) (string, string, error) {
	// Build list of repos not already in project
	existingPaths := make(map[string]bool)
	for _, r := range project.Repos {
		existingPaths[config.ExpandPath(r.Source)] = true
	}

	type repoOption struct {
		Name string
		Path string
	}
	var available []repoOption

	for name, path := range cfg.Repos {
		expanded := config.ExpandPath(path)
		if !existingPaths[expanded] {
			available = append(available, repoOption{Name: name, Path: expanded})
		}
	}

	if len(available) == 0 {
		return "", "", fmt.Errorf("all registered repos are already in this project")
	}

	// Build display items
	var items []string
	for _, r := range available {
		items = append(items, fmt.Sprintf("%s (%s)", r.Name, r.Path))
	}

	prompt := promptui.Select{
		Label: "Select repo to add",
		Items: items,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return "", "", err
	}

	selected := available[idx]
	return selected.Name, selected.Path, nil
}
