package cmd

import (
	"fmt"

	"github.com/daniil-lyalko/clade/internal/files"
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
	Use:        "feat [name]",
	Short:      "Create feature worktree (DEPRECATED: use 'work')",
	Deprecated: "use 'clade work' instead. 'feat' will be removed in v0.5.",
	Long: `DEPRECATED: Use 'clade work' instead.

Create a feature worktree for work you intend to merge.

Examples:
  clade work user-auth             # Use 'work' instead
  clade feat user-auth             # Still works but deprecated`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFeat,
}

func init() {
	rootCmd.AddCommand(featCmd)
	featCmd.Flags().StringVarP(&featRepoFlag, "repo", "r", "", "Repository path or registered name")
	featCmd.Flags().BoolVarP(&featPickFlag, "pick", "p", false, "Force repo picker even if in a git repo")
	featCmd.Flags().StringVarP(&featBranchFlag, "branch", "b", "", "Custom branch name (skips prompt)")
	featCmd.Flags().StringVarP(&featEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	featCmd.Flags().BoolVar(&featNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	featCmd.Flags().BoolVar(&featNoEditorFlag, "no-editor", false, "Skip opening the editor")
}

func runFeat(cmd *cobra.Command, args []string) error {
	// Show deprecation warning
	ui.Warn("'clade feat' is deprecated. Use 'clade work' instead.")
	fmt.Println()

	var name string
	var err error

	if len(args) > 0 {
		name = args[0]
	} else {
		prompt := promptui.Prompt{
			Label: "Feature name",
		}
		name, err = prompt.Run()
		if err != nil {
			return err
		}
	}

	// Delegate to the new feature command logic
	return CreateWorktree(name, GetBuiltInConfig("feature"), WorktreeOptions{
		RepoFlag:     featRepoFlag,
		PickFlag:     featPickFlag,
		BranchFlag:   featBranchFlag,
		EditorFlag:   featEditorFlag,
		NoAgentFlag:  featNoAgentFlag,
		NoEditorFlag: featNoEditorFlag,
	})
}

// Stub for files package usage (actual implementation in exp.go)
var _ = files.FindGitignored
