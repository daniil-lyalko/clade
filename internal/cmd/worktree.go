package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/pacer/internal/agent"
	"github.com/daniil-lyalko/pacer/internal/config"
	"github.com/daniil-lyalko/pacer/internal/files"
	"github.com/daniil-lyalko/pacer/internal/git"
	"github.com/daniil-lyalko/pacer/internal/hooks"
	"github.com/daniil-lyalko/pacer/internal/ui"
	"github.com/daniil-lyalko/pacer/internal/util"
	"github.com/manifoldco/promptui"
)

// WorktreeConfig holds the configuration for creating a worktree
type WorktreeConfig struct {
	Label        string // feature, bug, spike, chore, hotfix, docs, or custom
	BranchPrefix string // feat, fix, spike, chore, hotfix, docs
}

// WorktreeOptions holds the flags for worktree creation commands
type WorktreeOptions struct {
	RepoFlag     string // -r, --repo
	PickFlag     bool   // -p, --pick
	BranchFlag   string // -b, --branch
	FromFlag     string // -f, --from (base branch)
	EditorFlag   string // -o, --open
	AgentFlag    string // -a, --agent
	NoAgentFlag  bool   // --no-agent
	NoEditorFlag bool   // --no-editor
	DryRunFlag   bool   // --dry-run
}

// CreateWorktree creates a new worktree with the given configuration
func CreateWorktree(name string, wtCfg WorktreeConfig, opts WorktreeOptions) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate name
	if !util.IsValidName(name) {
		return fmt.Errorf("invalid name: use alphanumeric, hyphens, underscores only")
	}

	// Resolve repository
	repoPath, err := resolveRepoWithPick(cfg, opts.RepoFlag, opts.PickFlag)
	if err != nil {
		return err
	}

	// Update last used repo
	cfg.LastRepo = repoPath
	if err := cfg.Save(); err != nil {
		ui.Warn("Failed to save config: %v", err)
	}

	repoName := git.GetRepoName(repoPath)

	// Determine branch name
	var branch string
	if opts.BranchFlag != "" {
		branch = opts.BranchFlag
	} else {
		// Build default branch name - with or without prefix
		var defaultBranch string
		if wtCfg.BranchPrefix != "" {
			defaultBranch = wtCfg.BranchPrefix + "/" + name
		} else {
			defaultBranch = name
		}
		prompt := promptui.Prompt{
			Label:   "Branch name",
			Default: defaultBranch,
		}
		branch, err = prompt.Run()
		if err != nil {
			return err
		}
		if branch == "" {
			branch = defaultBranch
		}
	}

	// Load state
	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Check for existing worktree in v2 state
	if existing := state.GetWorktree(repoName, name); existing != nil {
		ui.Warn("Worktree '%s' already exists for %s", name, repoName)
		wtPath := config.WorktreePath(cfg, repoName, name)
		ui.KeyValue("Path", wtPath)

		prompt := promptui.Prompt{
			Label:     "Resume existing worktree",
			IsConfirm: true,
		}
		_, err := prompt.Run()
		if err == nil {
			return launchWorktreeSession(cfg, repoPath, wtPath, opts)
		}
		return nil
	}

	// Also check v1 experiments for backward compatibility
	expKey := config.ExperimentKey(repoPath, name)
	if existing := state.GetExperiment(expKey); existing != nil {
		ui.Warn("Worktree '%s' already exists (legacy format)", name)
		ui.KeyValue("Path", existing.Path)

		prompt := promptui.Prompt{
			Label:     "Resume existing worktree",
			IsConfirm: true,
		}
		_, err := prompt.Run()
		if err == nil {
			return launchWorktreeSession(cfg, repoPath, existing.Path, opts)
		}
		return nil
	}

	// v2 path: ~/pacer/repos/{repoName}/{name}/
	wtPath := config.WorktreePath(cfg, repoName, name)

	// Dry-run mode: show what would be created and exit
	if opts.DryRunFlag {
		return printDryRun(cfg, repoPath, repoName, name, branch, wtPath, wtCfg, opts)
	}

	// Create output
	ui.Header("Creating %s: %s", wtCfg.Label, name)
	ui.KeyValue("Repo", repoName)
	ui.KeyValue("Path", wtPath)
	ui.KeyValue("Branch", branch)

	// Ensure repos directory exists
	reposDir := filepath.Join(cfg.ReposDir(), repoName)
	if err := os.MkdirAll(reposDir, 0755); err != nil {
		return fmt.Errorf("failed to create repos directory: %w", err)
	}

	// Check if branch already exists
	ui.Info("Checking branch availability...")
	branchInfo := git.CheckBranch(repoPath, branch)
	if branchInfo.Status != git.BranchNotFound {
		ui.Error("Branch '%s' already exists", branch)
		ui.Detail("Use: pacer resume %s", name)
		ui.Detail("Or pick a different name")
		return fmt.Errorf("branch already exists")
	}

	// Create worktree with new branch
	ui.Info("Creating worktree...")
	baseBranch, err := git.CreateWorktreeNew(repoPath, wtPath, branch, opts.FromFlag)
	if err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}
	ui.Detail("Based on: %s", baseBranch)

	// Copy .claude/ directory if it exists in source repo
	sourceClaudeDir := filepath.Join(repoPath, ".claude")
	if _, err := os.Stat(sourceClaudeDir); err == nil {
		ui.Info("Copying .claude/ configuration...")
		if err := util.CopyDir(sourceClaudeDir, filepath.Join(wtPath, ".claude")); err != nil {
			ui.Warn("Failed to copy .claude/ directory: %v", err)
		}
	}

	// Copy .cursor/ directory if it exists in source repo
	sourceCursorDir := filepath.Join(repoPath, ".cursor")
	if _, err := os.Stat(sourceCursorDir); err == nil {
		ui.Info("Copying .cursor/ configuration...")
		if err := util.CopyDir(sourceCursorDir, filepath.Join(wtPath, ".cursor")); err != nil {
			ui.Warn("Failed to copy .cursor/ directory: %v", err)
		}
	}

	// Ensure required config files exist (.claude/ and .cursor/)
	if cfg.AutoInit {
		if err := EnsureAgentConfig(wtPath); err != nil {
			ui.Warn("Failed to ensure agent config: %v", err)
		}
	}

	// Copy gitignored files (.env, .npmrc, etc.)
	if copyGitignoredFilesImpl != nil {
		if err := copyGitignoredFilesImpl(cfg, repoPath, wtPath); err != nil {
			ui.Warn("Failed to copy some files: %v", err)
		}
	}

	// Create .pacer/metadata.json
	ticket := util.ExtractTicket(name)
	pacerMetadata := map[string]interface{}{
		"type":    wtCfg.Label,
		"name":    name,
		"ticket":  ticket,
		"repo":    repoName,
		"branch":  branch,
		"created": time.Now().Format(time.RFC3339),
	}
	metadataPath := filepath.Join(wtPath, ".pacer", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0755); err != nil {
		ui.Warn("Failed to create .pacer directory: %v", err)
	}
	if err := util.WriteJSON(metadataPath, pacerMetadata); err != nil {
		ui.Warn("Failed to write metadata.json: %v", err)
	}

	// Update state (v2 format)
	wt := &config.Worktree{
		Name:     name,
		Label:    wtCfg.Label,
		Branch:   branch,
		Ticket:   ticket,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}
	state.AddWorktree(repoName, wt)
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	ui.Success("Worktree created!")

	// Run on_create hooks
	if hooks.HasHooks(hooks.OnCreate, repoPath) {
		ui.Info("Running on_create hooks...")
		hookEnv := &hooks.Env{
			Type:     wtCfg.Label,
			Name:     name,
			Path:     wtPath,
			RepoName: repoName,
			RepoPath: repoPath,
			Branch:   branch,
			Ticket:   ticket,
		}
		results := hooks.RunHooks(hooks.OnCreate, hookEnv)
		for _, r := range results {
			if r.Error != nil {
				ui.Warn("Hook failed: %s - %v", r.Command, r.Error)
			} else {
				ui.Detail("Hook ran: %s", r.Command)
			}
		}
	}

	// Launch editor and/or agent
	return launchWorktreeSession(cfg, repoPath, wtPath, opts)
}

// launchWorktreeSession opens editor and/or launches agent
func launchWorktreeSession(cfg *config.Config, repoPath, wtPath string, opts WorktreeOptions) error {
	// Determine editor to use
	editor := cfg.Editor
	if opts.EditorFlag != "" {
		editor = opts.EditorFlag
	}

	// Open editor first (if configured and not disabled)
	if !opts.NoEditorFlag && editor != "" {
		editorOpts := agent.EditorOptions{
			TmuxSplitDirection: cfg.TmuxSplitDirection,
		}
		if err := agent.OpenEditor(wtPath, editor, editorOpts); err != nil {
			ui.Warn("Could not open editor: %s", err)
		} else {
			ui.Info("Opened %s", editor)
		}
	}

	// Determine agent to use
	agentCmd := cfg.Agent
	if opts.AgentFlag != "" {
		agentCmd = opts.AgentFlag
	}

	// Launch agent (if configured and not disabled)
	if !opts.NoAgentFlag && agentCmd != "" {
		ui.Info("Launching %s...", agentCmd)
		fmt.Println()

		ag := agent.NewAgent(agentCmd)
		agentOpts := agent.LaunchOptions{
			Flags: cfg.AgentFlags,
		}
		return ag.Launch(wtPath, agentOpts)
	}

	return nil
}

// ResumeWorktree resumes an existing worktree
func ResumeWorktree(cfg *config.Config, state *config.State, repoName string, wt *config.Worktree, repoPath string, opts WorktreeOptions) error {
	wtPath := config.GetWorktreePath(cfg, repoName, wt)

	// Verify path exists
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		ui.Error("Path no longer exists: %s", wtPath)
		ui.Detail("The worktree may have been removed manually")
		ui.Detail("Run: pacer cleanup %s", wt.Name)
		return fmt.Errorf("worktree not found")
	}

	// Ensure agent config files exist (auto-upgrade existing worktrees with new features)
	if err := EnsureAgentConfig(wtPath); err != nil {
		ui.Warn("Failed to ensure agent config: %v", err)
	}

	// Check for divergence if remote exists
	git.Fetch(repoPath)
	branchInfo := git.CheckBranch(repoPath, wt.Branch)
	if branchInfo.Diverged {
		ui.Warn("Branch diverged from origin (%d local, %d remote commits)", branchInfo.LocalAhead, branchInfo.RemoteBehind)
		ui.Detail("Resolve in worktree: git pull --rebase OR git merge")
	} else if branchInfo.RemoteBehind > 0 {
		ui.Info("Remote has %d new commits - consider: git pull", branchInfo.RemoteBehind)
	}

	// Update last used
	wt.LastUsed = time.Now()
	state.AddWorktree(repoName, wt)
	state.Save(cfg)

	ui.Header("Resuming: %s", wt.Name)
	ui.KeyValue("Path", wtPath)

	// Run on_resume hooks
	if hooks.HasHooks(hooks.OnResume, repoPath) {
		ui.Info("Running on_resume hooks...")
		hookEnv := &hooks.Env{
			Type:     wt.Label,
			Name:     wt.Name,
			Path:     wtPath,
			RepoName: repoName,
			RepoPath: repoPath,
			Branch:   wt.Branch,
			Ticket:   wt.Ticket,
		}
		results := hooks.RunHooks(hooks.OnResume, hookEnv)
		for _, r := range results {
			if r.Error != nil {
				ui.Warn("Hook failed: %s - %v", r.Command, r.Error)
			}
		}
	}

	return launchWorktreeSession(cfg, repoPath, wtPath, opts)
}

// GetBuiltInConfig returns the WorktreeConfig for a built-in label
func GetBuiltInConfig(label string) WorktreeConfig {
	labelCfg := config.BuiltInLabels()[label]
	return WorktreeConfig{
		Label:        label,
		BranchPrefix: labelCfg.BranchPrefix,
	}
}

// copyGitignoredFiles is re-exported from exp.go for use in worktree.go
// The actual implementation remains in exp.go to avoid circular dependencies
var copyGitignoredFilesImpl func(cfg *config.Config, srcRepo, dstPath string) error

// RegisterCopyGitignoredFiles registers the copyGitignoredFiles implementation
func RegisterCopyGitignoredFiles(fn func(cfg *config.Config, srcRepo, dstPath string) error) {
	copyGitignoredFilesImpl = fn
}

func init() {
	RegisterCopyGitignoredFiles(func(cfg *config.Config, srcRepo, dstPath string) error {
		// Build the file list: defaults + global config extras + per-repo extras
		patterns := make([]string, 0, len(files.DefaultCopyFiles)+len(cfg.CopyFiles))
		patterns = append(patterns, files.DefaultCopyFiles...)
		patterns = append(patterns, cfg.CopyFiles...)

		// Add per-repo extras if configured
		if repoFiles := cfg.GetRepoCopyFiles(srcRepo); len(repoFiles) > 0 {
			patterns = append(patterns, repoFiles...)
		}

		// Find which files actually exist in the source repo
		found := files.FindCopyableFiles(srcRepo, patterns)
		if len(found) == 0 {
			return nil
		}

		// Auto-copy without prompting
		ui.Info("Copying config files...")
		if err := files.CopyFiles(srcRepo, dstPath, found); err != nil {
			return err
		}
		for _, f := range found {
			ui.Detail("  Copied %s", f)
		}

		return nil
	})
}

// printDryRun shows what would be created without making changes
func printDryRun(cfg *config.Config, repoPath, repoName, name, branch, wtPath string, wtCfg WorktreeConfig, opts WorktreeOptions) error {
	fmt.Println()
	fmt.Println(ui.Bold("Dry run - no changes will be made"))
	fmt.Println()

	ui.KeyValue("Repository", fmt.Sprintf("%s (%s)", repoName, repoPath))
	ui.KeyValue("Worktree path", wtPath)
	ui.KeyValue("Branch", branch)
	ui.KeyValue("Label", wtCfg.Label)

	// Check what files would be copied
	patterns := make([]string, 0, len(files.DefaultCopyFiles)+len(cfg.CopyFiles))
	patterns = append(patterns, files.DefaultCopyFiles...)
	patterns = append(patterns, cfg.CopyFiles...)
	if repoFiles := cfg.GetRepoCopyFiles(repoPath); len(repoFiles) > 0 {
		patterns = append(patterns, repoFiles...)
	}
	found := files.FindCopyableFiles(repoPath, patterns)
	if len(found) > 0 {
		ui.KeyValue("Files to copy", fmt.Sprintf("%v", found))
	}

	// Check what would be run
	fmt.Println()
	fmt.Println(ui.Dim("Would run:"))
	fmt.Printf("  git worktree add %s -b %s\n", wtPath, branch)
	if cfg.AutoInit {
		fmt.Println("  pacer init (auto_init enabled)")
	}

	// Show agent/editor that would launch
	editor := cfg.Editor
	if opts.EditorFlag != "" {
		editor = opts.EditorFlag
	}
	agentCmd := cfg.Agent
	if opts.AgentFlag != "" {
		agentCmd = opts.AgentFlag
	}

	if !opts.NoEditorFlag && editor != "" {
		fmt.Printf("  Open: %s\n", editor)
	}
	if !opts.NoAgentFlag && agentCmd != "" {
		fmt.Printf("  Launch: %s\n", agentCmd)
	}

	fmt.Println()
	fmt.Println(ui.Dim("Remove --dry-run to execute"))
	return nil
}
