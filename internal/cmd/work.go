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
	workEditorFlag   string
	workAgentFlag    string
	workNoAgentFlag  bool
	workNoEditorFlag bool
	workTypeFlag     string
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

Examples:
  clade work new-api              # branch: new-api (no prefix)
  clade work try-redis -t spike   # branch: spike/try-redis
  clade work PROJ-123 -t bug      # branch: fix/PROJ-123
  clade work cleanup -t chore     # branch: chore/cleanup
  clade work foo -t perf          # custom label from config

Creates:
  - A new worktree at ~/clade/repos/{repo}/{name}/
  - A branch (with optional type prefix)
  - Copies .claude/ config from the source repo`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWork,
}

func init() {
	rootCmd.AddCommand(workCmd)
	workCmd.Flags().StringVarP(&workTypeFlag, "type", "t", "", "Type of worktree (spike, feature, bug, chore, hotfix, docs, or custom)")
	workCmd.Flags().StringVarP(&workRepoFlag, "repo", "r", "", "Repository path or registered name")
	workCmd.Flags().BoolVarP(&workPickFlag, "pick", "p", false, "Force repo picker")
	workCmd.Flags().StringVarP(&workBranchFlag, "branch", "b", "", "Custom branch name")
	workCmd.Flags().StringVarP(&workEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	workCmd.Flags().StringVarP(&workAgentFlag, "agent", "a", "", "Override configured agent")
	workCmd.Flags().BoolVar(&workNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	workCmd.Flags().BoolVar(&workNoEditorFlag, "no-editor", false, "Skip opening the editor")
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
		EditorFlag:   workEditorFlag,
		AgentFlag:    workAgentFlag,
		NoAgentFlag:  workNoAgentFlag,
		NoEditorFlag: workNoEditorFlag,
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
