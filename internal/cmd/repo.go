package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var repoNameFlag string

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage registered repositories",
	Long:  `Register, list, and remove repositories for quick access from anywhere.`,
}

var repoAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Register a repository or folder of repositories",
	Long: `Register a repository for quick access from anywhere.

If the path is a git repository, it will be registered directly.
If the path is a directory containing git repositories, all repos
in that directory will be registered.

Examples:
  clade repo add ~/repos/my-project
  clade repo add . --name backend
  clade repo add ~/repos/api --name api
  clade repo add ~/repos              # Scans and adds all repos in folder`,
	Args: cobra.ExactArgs(1),
	RunE: runRepoAdd,
}

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered repositories",
	RunE:  runRepoList,
}

var repoRemoveCmd = &cobra.Command{
	Use:               "remove <name>",
	Short:             "Unregister a repository",
	Args:              cobra.ExactArgs(1),
	RunE:              runRepoRemove,
	ValidArgsFunction: completeRepoNames,
}

func init() {
	rootCmd.AddCommand(repoCmd)
	repoCmd.AddCommand(repoAddCmd)
	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoRemoveCmd)

	repoAddCmd.Flags().StringVar(&repoNameFlag, "name", "", "Custom name for the repository")
}

func runRepoAdd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	path := config.ExpandPath(args[0])

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if it's a git repo directly
	if git.IsGitRepo(absPath) {
		return addSingleRepo(cfg, absPath, repoNameFlag)
	}

	// Not a git repo - check if it's a directory we can scan
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path not found: %s", absPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a git repository: %s", absPath)
	}

	// Scan for git repos in subdirectories
	return scanAndAddRepos(cfg, absPath)
}

func addSingleRepo(cfg *config.Config, absPath, customName string) error {
	// Get repo root
	repoRoot, err := git.GetRepoRoot(absPath)
	if err != nil {
		return err
	}

	// Determine name
	name := customName
	if name == "" {
		name = filepath.Base(repoRoot)
	}

	// Check if name already exists
	if existing, ok := cfg.Repos[name]; ok {
		if config.ExpandPath(existing) == repoRoot {
			ui.Info("Repository '%s' is already registered", name)
			return nil
		}
		return fmt.Errorf("name '%s' already registered for %s", name, existing)
	}

	// Add to config
	cfg.Repos[name] = repoRoot
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ui.Success("Registered repository '%s'", name)
	ui.KeyValue("Path", repoRoot)

	return nil
}

func scanAndAddRepos(cfg *config.Config, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	var added, skipped, alreadyRegistered int

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		subdir := filepath.Join(dir, entry.Name())
		if !git.IsGitRepo(subdir) {
			continue
		}

		repoRoot, err := git.GetRepoRoot(subdir)
		if err != nil {
			skipped++
			continue
		}

		name := filepath.Base(repoRoot)

		// Check if already registered
		if existing, ok := cfg.Repos[name]; ok {
			if config.ExpandPath(existing) == repoRoot {
				alreadyRegistered++
				continue
			}
			// Name conflict - skip with unique suffix note
			ui.Warn("Skipped '%s' - name already used for %s", name, existing)
			skipped++
			continue
		}

		cfg.Repos[name] = repoRoot
		added++
	}

	if added == 0 && alreadyRegistered == 0 && skipped == 0 {
		return fmt.Errorf("no git repositories found in %s", dir)
	}

	if added > 0 {
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		ui.Success("Registered %d repositories", added)
	}

	if alreadyRegistered > 0 {
		ui.Info("%d already registered", alreadyRegistered)
	}

	if skipped > 0 {
		ui.Warn("%d skipped (conflicts or errors)", skipped)
	}

	return nil
}

func runRepoList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.Repos) == 0 {
		ui.Info("No repositories registered")
		ui.Detail("Use: clade repo add <path>")
		return nil
	}

	ui.Header("Registered repositories:")
	for name, path := range cfg.Repos {
		suffix := ""
		if config.ExpandPath(path) == cfg.LastRepo {
			suffix = ui.Dim(" (last used)")
		}
		fmt.Printf("  %s%s\n", ui.Cyan(name), suffix)
		fmt.Printf("    %s\n", ui.Dim(path))
	}

	return nil
}

func runRepoRemove(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	name := args[0]

	if _, ok := cfg.Repos[name]; !ok {
		return fmt.Errorf("repository '%s' not found", name)
	}

	delete(cfg.Repos, name)

	// Clear last repo if it was the one we removed
	if cfg.LastRepo != "" {
		found := false
		for _, path := range cfg.Repos {
			if config.ExpandPath(path) == cfg.LastRepo {
				found = true
				break
			}
		}
		if !found {
			cfg.LastRepo = ""
		}
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ui.Success("Removed repository '%s'", name)

	return nil
}

// completeRepoNames provides shell completion for registered repo names
func completeRepoNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for name, path := range cfg.Repos {
		names = append(names, name+"\t"+path)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}
