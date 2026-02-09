package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	workRepoFlag     string
	workPickFlag     bool
	workBranchFlag   string
	workFromFlag     string
	workEditorFlag   string
	workAgentFlag    string
	workNoAgentFlag  bool
	workNoEditorFlag bool
	workTypeFlag     string
	workDryRunFlag   bool
)

var workCmd = &cobra.Command{
	Use:   "work [name]",
	Short: "Create a new worktree",
	Long: `Create a worktree for isolated development.

By default, creates a worktree with no branch prefix (branch name = worktree name).
Use -t to add a type/prefix:

  spike    Throwaway experiment (spike/ branch)
  feature  New functionality (feat/ branch)
  bug      Bug fix (fix/ branch)
  chore    Maintenance/refactor (chore/ branch)
  hotfix   Urgent fix (hotfix/ branch)
  docs     Documentation (docs/ branch)

Custom labels defined in config are also available.

By default, new branches are created from origin's default branch (main/master).
Use -f/--from to specify a different base branch:

  clade work foo --from develop   # branch from develop instead of main

Examples:
  clade work new-api              # branch: new-api (from main)
  clade work try-redis -t spike   # branch: spike/try-redis (from main)
  clade work PROJ-123 -t bug      # branch: fix/PROJ-123 (from main)
  clade work foo -f develop       # branch: foo (from develop)
  clade work foo -t perf          # custom label from config

Creates:
  - A new worktree at ~/clade/repos/{repo}/{name}/
  - A branch (with optional type prefix, from specified base)
  - Copies .claude/ config from the source repo`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWork,
}

func init() {
	rootCmd.AddCommand(workCmd)
	// Hide work command - users should use `clade <name>` shorthand instead
	workCmd.Hidden = true
	// Command-specific flags (editor/agent flags inherited from root's persistent flags)
	workCmd.Flags().StringVarP(&workTypeFlag, "type", "t", "", "Type of worktree (spike, feature, bug, chore, hotfix, docs, or custom)")
	workCmd.Flags().StringVarP(&workRepoFlag, "repo", "r", "", "Repository path or registered name")
	workCmd.Flags().BoolVarP(&workPickFlag, "pick", "p", false, "Force repo picker")
	workCmd.Flags().StringVarP(&workBranchFlag, "branch", "b", "", "Custom branch name")
	workCmd.Flags().StringVarP(&workFromFlag, "from", "f", "", "Base branch to create from (default: origin's default branch)")
	workCmd.Flags().BoolVar(&workDryRunFlag, "dry-run", false, "Preview what would be created without making changes")
}

func runWork(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Handle type/label - empty means no prefix (plain worktree)
	var label string
	var branchPrefix string

	if workTypeFlag == "" {
		// No type specified - use no prefix, label as "worktree"
		label = "worktree"
		branchPrefix = ""
	} else {
		// Validate type/label exists
		labelCfg, ok := cfg.GetLabelConfig(workTypeFlag)
		if !ok {
			return fmt.Errorf("unknown type '%s'. %s", workTypeFlag, availableTypesHelp(cfg))
		}
		label = workTypeFlag
		branchPrefix = labelCfg.BranchPrefix
	}

	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		promptLabel := "Worktree name"
		if workTypeFlag != "" {
			promptLabel = fmt.Sprintf("%s name", strings.Title(workTypeFlag))
		}
		prompt := promptui.Prompt{
			Label: promptLabel,
		}
		name, err = prompt.Run()
		if err != nil {
			return err
		}
	}

	return CreateWorktree(name, WorktreeConfig{
		Label:        label,
		BranchPrefix: branchPrefix,
	}, WorktreeOptions{
		RepoFlag:     workRepoFlag,
		PickFlag:     workPickFlag,
		BranchFlag:   workBranchFlag,
		FromFlag:     workFromFlag,
		EditorFlag:   workEditorFlag,
		AgentFlag:    workAgentFlag,
		NoAgentFlag:  workNoAgentFlag,
		NoEditorFlag: workNoEditorFlag,
		DryRunFlag:   workDryRunFlag,
	})
}

// availableTypesHelp returns a formatted help string listing available types
func availableTypesHelp(cfg *config.Config) string {
	var builtIn []string
	for label := range config.BuiltInLabels() {
		builtIn = append(builtIn, label)
	}
	sort.Strings(builtIn)

	var custom []string
	for label := range cfg.CustomLabels {
		custom = append(custom, label)
	}
	sort.Strings(custom)

	msg := fmt.Sprintf("Available types: %s", strings.Join(builtIn, ", "))
	if len(custom) > 0 {
		msg += fmt.Sprintf("\nCustom types: %s", strings.Join(custom, ", "))
	}
	return msg
}
