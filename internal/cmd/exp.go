package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	expRepoFlag   string
	expAgentFlag  string
)

var expCmd = &cobra.Command{
	Use:   "exp [name]",
	Short: "Create isolated experiment worktree",
	Long: `Create an isolated experiment worktree and launch your AI agent.

Examples:
  clade exp try-redis           # Quick experiment
  clade exp PROJ-1234           # Ticket investigation
  clade exp fix-auth -r backend # Specify repo by name

The experiment creates:
  - A new worktree at ~/clade/experiments/{repo}-{name}/
  - A branch named exp/{name}
  - Copies .claude/ config from the source repo`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExp,
}

func init() {
	rootCmd.AddCommand(expCmd)
	expCmd.Flags().StringVarP(&expRepoFlag, "repo", "r", "", "Repository path or registered name")
	expCmd.Flags().StringVarP(&expAgentFlag, "agent", "a", "", "Agent to launch (overrides config)")
}

func runExp(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get experiment name
	var expName string
	if len(args) > 0 {
		expName = args[0]
	} else {
		prompt := promptui.Prompt{
			Label: "Experiment name",
		}
		expName, err = prompt.Run()
		if err != nil {
			return err
		}
	}

	// Validate experiment name
	if !isValidExpName(expName) {
		return fmt.Errorf("invalid experiment name: use alphanumeric, hyphens, underscores only")
	}

	// Resolve repository
	repoPath, err := resolveRepo(cfg, expRepoFlag)
	if err != nil {
		return err
	}

	// Update last used repo
	cfg.LastRepo = repoPath
	if err := cfg.Save(); err != nil {
		ui.Warn("Failed to save config: %v", err)
	}

	repoName := git.GetRepoName(repoPath)
	expKey := config.ExperimentKey(repoPath, expName)
	expPath := filepath.Join(cfg.ExperimentsDir(), expKey)
	branch := "exp/" + expName

	// Check if experiment already exists
	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if existing := state.GetExperiment(expKey); existing != nil {
		ui.Warn("Experiment '%s' already exists", expName)
		ui.KeyValue("Path", existing.Path)

		prompt := promptui.Prompt{
			Label:     "Resume existing experiment",
			IsConfirm: true,
		}
		_, err := prompt.Run()
		if err == nil {
			// User wants to resume
			return launchAgent(cfg, existing.Path, expAgentFlag)
		}
		return nil
	}

	// Create experiment directory
	ui.Header("Creating experiment: %s", expName)
	ui.KeyValue("Repo", repoName)
	ui.KeyValue("Path", expPath)
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
		ui.Detail("Use: clade resume %s", expName)
		ui.Detail("Or pick a different name")
		return fmt.Errorf("branch already exists")
	}

	// Create worktree with new branch from origin's default
	ui.Info("Creating worktree...")
	if err := git.CreateWorktreeNew(repoPath, expPath, branch); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	// Copy .claude/ directory if it exists in source repo
	sourceClaudeDir := filepath.Join(repoPath, ".claude")
	if _, err := os.Stat(sourceClaudeDir); err == nil {
		ui.Info("Copying .claude/ configuration...")
		if err := copyDir(sourceClaudeDir, filepath.Join(expPath, ".claude")); err != nil {
			ui.Warn("Failed to copy .claude/ directory: %v", err)
		}
	} else if cfg.AutoInit {
		// No .claude/ in source, auto-initialize
		ui.Info("Initializing .claude/ configuration...")
		if err := InitRepo(expPath); err != nil {
			ui.Warn("Failed to initialize .claude/: %v", err)
		}
	}

	// Copy gitignored files (.env, .npmrc, etc.)
	if err := copyGitignoredFiles(cfg, repoPath, expPath); err != nil {
		ui.Warn("Failed to copy some files: %v", err)
	}

	// Create .clade.json metadata
	ticket := extractTicket(expName)
	cladeMetadata := map[string]interface{}{
		"type":    "experiment",
		"name":    expName,
		"ticket":  ticket,
		"repo":    repoName,
		"created": time.Now().Format(time.RFC3339),
	}
	if err := writeJSON(filepath.Join(expPath, ".clade.json"), cladeMetadata); err != nil {
		ui.Warn("Failed to write .clade.json: %v", err)
	}

	// Update state
	exp := &config.Experiment{
		Name:     expName,
		Repo:     repoPath,
		Path:     expPath,
		Branch:   branch,
		Ticket:   ticket,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}
	state.AddExperiment(exp)
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	ui.Success("Experiment created!")

	// Launch agent
	return launchAgent(cfg, expPath, expAgentFlag)
}

func resolveRepo(cfg *config.Config, repoFlag string) (string, error) {
	// 1. Check if repo flag was provided
	if repoFlag != "" {
		// Check if it's a registered repo name
		if path, ok := cfg.Repos[repoFlag]; ok {
			return config.ExpandPath(path), nil
		}
		// Assume it's a path
		expanded := config.ExpandPath(repoFlag)
		if git.IsGitRepo(expanded) {
			return git.GetRepoRoot(expanded)
		}
		return "", fmt.Errorf("not a git repository: %s", repoFlag)
	}

	// 2. Check if we're in a git repo
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if git.IsGitRepo(cwd) {
		return git.GetRepoRoot(cwd)
	}

	// 3. Check if there are registered repos
	if len(cfg.Repos) == 0 {
		return "", fmt.Errorf("not in a git repo. Register repos with: clade repo add <path>")
	}

	// 4. Interactive picker
	var repoNames []string
	// Put last used repo first
	if cfg.LastRepo != "" {
		for name, path := range cfg.Repos {
			if config.ExpandPath(path) == cfg.LastRepo {
				repoNames = append([]string{name + " (last used)"}, repoNames...)
			} else {
				repoNames = append(repoNames, name)
			}
		}
	} else {
		for name := range cfg.Repos {
			repoNames = append(repoNames, name)
		}
	}

	prompt := promptui.Select{
		Label: "Select repo",
		Items: repoNames,
	}
	_, selected, err := prompt.Run()
	if err != nil {
		return "", err
	}

	// Remove " (last used)" suffix if present
	selected = strings.TrimSuffix(selected, " (last used)")
	return config.ExpandPath(cfg.Repos[selected]), nil
}

func launchAgent(cfg *config.Config, workdir string, agentOverride string) error {
	agentCmd := cfg.Agent
	if agentOverride != "" {
		agentCmd = agentOverride
	}

	ui.Info("Launching %s...", agentCmd)
	fmt.Println()

	ag := agent.NewAgent(agentCmd)
	opts := agent.LaunchOptions{
		Flags: cfg.AgentFlags,
	}

	return ag.Launch(workdir, opts)
}

func isValidExpName(name string) bool {
	if name == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, name)
	return matched
}

// extractTicket extracts a JIRA-style ticket ID from the experiment name
func extractTicket(name string) string {
	// Match patterns like PROJ-1234, ABC-123, etc.
	re := regexp.MustCompile(`^([A-Z]+-\d+)`)
	matches := re.FindStringSubmatch(strings.ToUpper(name))
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

func writeJSON(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// copyGitignoredFiles handles interactive selection and copying of gitignored files
func copyGitignoredFiles(cfg *config.Config, srcRepo, dstPath string) error {
	// Check if we have saved preferences for this repo
	savedFiles := cfg.GetRepoCopyFiles(srcRepo)
	if savedFiles != nil {
		// Use saved preferences
		if len(savedFiles) > 0 {
			ui.Info("Copying saved file preferences...")
			if err := files.CopyFiles(srcRepo, dstPath, savedFiles); err != nil {
				return err
			}
			for _, f := range savedFiles {
				ui.Detail("  Copied %s", f)
			}
		}
		return nil
	}

	// No saved preferences - detect and prompt
	detected := files.FindGitignored(srcRepo)
	if len(detected) == 0 {
		return nil
	}

	fmt.Println()
	ui.Info("Found gitignored files in source repo:")
	for _, f := range detected {
		ui.Detail("  %s", f)
	}
	fmt.Println()

	// Interactive selection
	selected, err := selectFilesToCopy(detected)
	if err != nil {
		return nil // User cancelled, not an error
	}

	// Save preference for future
	cfg.SetRepoCopyFiles(srcRepo, selected)
	if err := cfg.Save(); err != nil {
		ui.Warn("Failed to save file preferences: %v", err)
	}

	// Copy selected files
	if len(selected) > 0 {
		ui.Info("Copying selected files...")
		if err := files.CopyFiles(srcRepo, dstPath, selected); err != nil {
			return err
		}
		for _, f := range selected {
			ui.Detail("  Copied %s", f)
		}
	}

	return nil
}

// selectFilesToCopy shows an interactive prompt to select files
func selectFilesToCopy(detected []string) ([]string, error) {
	// Simple approach: show list and ask which to copy
	// For now, use a yes/no per file approach

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

	// Ask if user wants to save this preference
	if len(detected) > 0 {
		fmt.Println()
		ui.Detail("These preferences will be saved for future experiments from this repo.")
	}

	return selected, nil
}
