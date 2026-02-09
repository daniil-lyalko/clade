package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/spf13/cobra"
)

var pathJSONFlag bool

var pathCmd = &cobra.Command{
	Use:   "path <name>",
	Short: "Print worktree path to stdout",
	Long: `Prints the absolute path of a worktree to stdout.

Useful for scripting:
  cd $(clade path foo)
  code $(clade path foo)

If --json is set, outputs {"path": "/absolute/path/to/worktree"}.
Exits with error if worktree not found.`,
	Args: cobra.ExactArgs(1),
	RunE: runPath,
}

func init() {
	rootCmd.AddCommand(pathCmd)
	pathCmd.Flags().BoolVar(&pathJSONFlag, "json", false, "Output as JSON")
}

func runPath(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Search worktrees across all repos
	for repoName := range cfg.Repos {
		if wt := state.GetWorktree(repoName, name); wt != nil {
			path := config.WorktreePath(cfg, repoName, name)
			if pathJSONFlag {
				return json.NewEncoder(os.Stdout).Encode(map[string]string{"path": path})
			}
			fmt.Println(path)
			return nil
		}
	}

	// Check legacy experiments
	for _, exp := range state.Experiments {
		if exp.Name == name {
			if pathJSONFlag {
				return json.NewEncoder(os.Stdout).Encode(map[string]string{"path": exp.Path})
			}
			fmt.Println(exp.Path)
			return nil
		}
	}

	// Check scratches
	for _, scratch := range state.Scratches {
		if scratch.Name == name {
			if pathJSONFlag {
				return json.NewEncoder(os.Stdout).Encode(map[string]string{"path": scratch.Path})
			}
			fmt.Println(scratch.Path)
			return nil
		}
	}

	return fmt.Errorf("worktree '%s' not found", name)
}
