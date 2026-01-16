package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/files"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	featRepoFlag     string
	featPickFlag     bool
	featBranchFlag   string
	featEditorFlag   string
	featNoAgentFlag  bool
	featNoEditorFlag bool
)

var featCmd = &cobra.Command{
	Use:   "feat [name]",
	Short: "Create feature worktree (intended to merge)",
	Long: `Create a feature worktree for work you intend to merge.

Unlike 'exp' (experiments/spikes that may be thrown away), 'feat' is for
features you plan to merge into the main branch.

Examples:
  clade feat user-auth             # New feature
  clade feat PROJ-1234             # Ticket-based feature
  clade feat fix-auth -r backend   # Specify repo by name
  clade feat fix-auth -p           # Force repo picker
  clade feat foo -b custom/branch  # Custom branch name
  clade feat foo -o cursor         # Open Cursor IDE
  clade feat foo --no-agent        # Skip launching Claude

The feature creates:
  - A new worktree at ~/clade/experiments/{repo}-{name}/
  - A branch (default: feat/{name}, or custom with -b)
  - Copies .claude/ config from the source repo`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFeat,
}

func init() {
	rootCmd.AddCommand(featCmd)
	featCmd.Flags().StringVarP(&featRepoFlag, "repo", "r", "", "Repository path or registered name")
	featCmd.Flags().BoolVarP(&featPickFlag, "pick", "p", false, "Force repo picker even if in a git repo")
	featCmd.Flags().StringVarP(&featBranchFlag, "branch", "b", "", "Custom branch name (skips prompt)")
	featCmd.Flags().StringVarP(&featEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	featCmd.Flags().StringVarP(&featEditorFlag, "editor", "e", "", "Alias for --open")
	featCmd.Flags().BoolVar(&featNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	featCmd.Flags().BoolVar(&featNoEditorFlag, "no-editor", false, "Skip opening the editor")
}

func runFeat(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get feature name
	var featName string
	if len(args) > 0 {
		featName = args[0]
	} else {
		prompt := promptui.Prompt{
			Label: "Feature name",
		}
		featName, err = prompt.Run()
		if err != nil {
			return err
		}
	}

	// Validate feature name (reuse exp validation)
	if !isValidExpName(featName) {
		return fmt.Errorf("invalid feature name: use alphanumeric, hyphens, underscores only")
	}

	// Resolve repository (with pick flag support)
	repoPath, err := resolveRepoWithPick(cfg, featRepoFlag, featPickFlag)
	if err != nil {
		return err
	}

	// Update last used repo
	cfg.LastRepo = repoPath
	if err := cfg.Save(); err != nil {
		ui.Warn("Failed to save config: %v", err)
	}

	repoName := git.GetRepoName(repoPath)
	// Use same key format as experiments (stored in same place)
	expKey := config.ExperimentKey(repoPath, featName)
	featPath := filepath.Join(cfg.ExperimentsDir(), expKey)

	// Get branch name (prompt if not provided via flag)
	var branch string
	if featBranchFlag != "" {
		branch = featBranchFlag
	} else {
		defaultBranch := "feat/" + featName
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

	// Check if feature already exists
	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if existing := state.GetExperiment(expKey); existing != nil {
		ui.Warn("Feature '%s' already exists", featName)
		ui.KeyValue("Path", existing.Path)

		prompt := promptui.Prompt{
			Label:     "Resume existing feature",
			IsConfirm: true,
		}
		_, err := prompt.Run()
		if err == nil {
			// User wants to resume
			return launchSession(cfg, existing.Path, featEditorFlag, featNoAgentFlag, featNoEditorFlag)
		}
		return nil
	}

	// Create feature directory
	ui.Header("Creating feature: %s", featName)
	ui.KeyValue("Repo", repoName)
	ui.KeyValue("Path", featPath)
	ui.KeyValue("Branch", branch)

	// Ensure experiments directory exists
	if err := os.MkdirAll(cfg.ExperimentsDir(), 0755); err != nil {
		return fmt.Errorf("failed to create experiments directory: %w", err)
	}

	// Check if branch already exists (local or remote)
	ui.Info("Checking branch availability...")
	branchInfo := git.CheckBranch(repoPath, branch)
	if branchInfo.Status != git.BranchNotFound {
		ui.Error("Branch '%s' already exists", branch)
		ui.Detail("Use: clade resume %s", featName)
		ui.Detail("Or pick a different name")
		return fmt.Errorf("branch already exists")
	}

	// Create worktree with new branch from origin's default
	ui.Info("Creating worktree...")
	if err := git.CreateWorktreeNew(repoPath, featPath, branch); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	// Copy .claude/ directory if it exists in source repo
	sourceClaudeDir := filepath.Join(repoPath, ".claude")
	if _, err := os.Stat(sourceClaudeDir); err == nil {
		ui.Info("Copying .claude/ configuration...")
		if err := copyDir(sourceClaudeDir, filepath.Join(featPath, ".claude")); err != nil {
			ui.Warn("Failed to copy .claude/ directory: %v", err)
		}
	} else if cfg.AutoInit {
		// No .claude/ in source, auto-initialize
		ui.Info("Initializing .claude/ configuration...")
		if err := InitRepo(featPath); err != nil {
			ui.Warn("Failed to initialize .claude/: %v", err)
		}
	}

	// Copy gitignored files (.env, .npmrc, etc.)
	if err := copyGitignoredFiles(cfg, repoPath, featPath); err != nil {
		ui.Warn("Failed to copy some files: %v", err)
	}

	// Create .clade.json metadata
	ticket := extractTicket(featName)
	cladeMetadata := map[string]any{
		"type":    "feature",
		"name":    featName,
		"ticket":  ticket,
		"repo":    repoName,
		"created": time.Now().Format(time.RFC3339),
	}
	if err := writeJSON(filepath.Join(featPath, ".clade.json"), cladeMetadata); err != nil {
		ui.Warn("Failed to write .clade.json: %v", err)
	}

	// Update state (stored as experiment for now - same storage)
	exp := &config.Experiment{
		Name:     featName,
		Repo:     repoPath,
		Path:     featPath,
		Branch:   branch,
		Ticket:   ticket,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}
	state.AddExperiment(exp)
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	ui.Success("Feature created!")

	// Launch editor and/or agent
	return launchSession(cfg, featPath, featEditorFlag, featNoAgentFlag, featNoEditorFlag)
}

// Stub for files package usage (actual implementation in exp.go)
var _ = files.FindGitignored
