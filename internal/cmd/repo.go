package cmd

import (
	"fmt"
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
	Short: "Register a repository",
	Long: `Register a repository for quick access from anywhere.

Examples:
  clade repo add ~/repos/my-project
  clade repo add . --name backend
  clade repo add ~/repos/api --name api`,
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

	// Verify it's a git repo
	if !git.IsGitRepo(absPath) {
		return fmt.Errorf("not a git repository: %s", absPath)
	}

	// Get repo root
	repoRoot, err := git.GetRepoRoot(absPath)
	if err != nil {
		return err
	}

	// Determine name
	name := repoNameFlag
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
