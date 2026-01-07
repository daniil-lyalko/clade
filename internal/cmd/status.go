package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/daniil-lyalko/clade/internal/context"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show context for current directory",
	Long: `Display clade context information for the current directory.

Shows:
  - Experiment/project metadata
  - Context files (CLAUDE.md, DROPBAG.md, TICKET.md)
  - Git status summary
  - Recent commits

Works in any directory:
  - In a clade experiment: Full context info
  - In a regular git repo: Basic info + suggestion to init
  - Not in git repo: Clear message`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Check if we're in a git repo
	if !git.IsGitRepo(cwd) {
		ui.Info("Not in a git repository")
		ui.Detail("Navigate to a git repo or clade experiment")
		return nil
	}

	repoRoot, err := git.GetRepoRoot(cwd)
	if err != nil {
		return err
	}

	// Check for .clade.json (indicates clade-managed worktree)
	cladeMetaPath := filepath.Join(repoRoot, ".clade.json")
	metadata, _ := context.ReadCladeMetadata(repoRoot)

	if metadata != nil {
		// We're in a clade-managed worktree
		printCladeStatus(repoRoot, metadata)
	} else if _, err := os.Stat(cladeMetaPath); os.IsNotExist(err) {
		// Regular git repo, not clade-managed
		printBasicStatus(repoRoot)
	}

	return nil
}

func printCladeStatus(repoRoot string, metadata *context.CladeMetadata) {
	// Header with type
	typeLabel := "Experiment"
	if metadata.Type == "project" {
		typeLabel = "Project"
	}
	ui.Header("%s: %s", typeLabel, metadata.Name)
	ui.KeyValue("Repo", metadata.Repo)

	// Branch
	if branch, err := git.GetCurrentBranch(repoRoot); err == nil {
		ui.KeyValue("Branch", branch)
	}
	ui.KeyValue("Type", metadata.Type)

	// Context files section
	fmt.Println()
	ui.Header("Context Files:")
	checkContextFile(repoRoot, "CLAUDE.md", "project context")
	checkContextFile(repoRoot, "DROPBAG.md", "session handoff notes")

	if metadata.Ticket != "" {
		checkContextFile(repoRoot, "TICKET.md", fmt.Sprintf("ticket %s", metadata.Ticket))
	} else {
		fmt.Printf("  %s %s %s\n", ui.Dim("○"), ui.Dim("TICKET.md"), ui.Dim("(no ticket linked)"))
	}

	// Check for .claude/ setup
	claudeSettingsPath := filepath.Join(repoRoot, ".claude", "settings.json")
	if _, err := os.Stat(claudeSettingsPath); err == nil {
		fmt.Printf("  %s %s %s\n", ui.Green("✓"), ".claude/", ui.Dim("(hooks configured)"))
	} else {
		fmt.Printf("  %s %s %s\n", ui.Yellow("○"), ".claude/", ui.Dim("(not initialized)"))
	}

	// Git status section
	fmt.Println()
	ui.Header("Git Status:")
	printGitStatus(repoRoot)

	// Recent commits
	fmt.Println()
	ui.Header("Recent Commits:")
	if commits, err := git.GetRecentCommits(repoRoot, 3); err == nil && len(commits) > 0 {
		for _, commit := range commits {
			fmt.Printf("  %s\n", commit)
		}
	} else {
		ui.Detail("No commits yet")
	}
}

func printBasicStatus(repoRoot string) {
	repoName := git.GetRepoName(repoRoot)

	ui.Header("Repository: %s", repoName)

	// Branch
	if branch, err := git.GetCurrentBranch(repoRoot); err == nil {
		ui.KeyValue("Branch", branch)
	}

	ui.KeyValue("Path", repoRoot)
	fmt.Printf("  %s\n", ui.Dim("(not a clade experiment)"))

	// Check if .claude/ exists
	fmt.Println()
	claudeSettingsPath := filepath.Join(repoRoot, ".claude", "settings.json")
	if _, err := os.Stat(claudeSettingsPath); err == nil {
		fmt.Printf("  %s %s\n", ui.Green("✓"), "Hooks configured (.claude/settings.json)")
	} else {
		fmt.Printf("  %s %s\n", ui.Yellow("○"), "No hooks configured")
		ui.Detail("Run 'clade init' to set up SessionStart hooks")
	}

	// Git status
	fmt.Println()
	ui.Header("Git Status:")
	printGitStatus(repoRoot)

	// Suggestion
	fmt.Println()
	ui.Info("This is a regular git repo, not a clade experiment")
	ui.Detail("Create an experiment: clade exp <name>")
	ui.Detail("Or initialize hooks: clade init")
}

func checkContextFile(repoRoot, filename, description string) {
	path := filepath.Join(repoRoot, filename)
	if info, err := os.Stat(path); err == nil {
		age := formatAge(info.ModTime())
		fmt.Printf("  %s %s %s\n", ui.Green("✓"), filename, ui.Dim(fmt.Sprintf("(%s, %s)", description, age)))
	} else {
		fmt.Printf("  %s %s %s\n", ui.Dim("○"), ui.Dim(filename), ui.Dim(fmt.Sprintf("(no %s)", description)))
	}
}

func printGitStatus(repoRoot string) {
	status, err := git.GetStatus(repoRoot)
	if err != nil {
		ui.Detail("Unable to get git status")
		return
	}

	if status.Clean {
		fmt.Printf("  %s\n", ui.Green("Working tree clean"))
		return
	}

	// Summary counts
	var parts []string
	if len(status.StagedFiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", len(status.StagedFiles)))
	}
	if len(status.ModifiedFiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", len(status.ModifiedFiles)))
	}
	if len(status.UntrackedFiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", len(status.UntrackedFiles)))
	}

	fmt.Printf("  %s\n", ui.Yellow(fmt.Sprintf("%d uncommitted changes", status.UncommittedCount)))

	// Show first few files
	shown := 0
	maxShow := 5

	for _, f := range status.StagedFiles {
		if shown >= maxShow {
			break
		}
		fmt.Printf("    %s %s\n", ui.Green("A"), f)
		shown++
	}
	for _, f := range status.ModifiedFiles {
		if shown >= maxShow {
			break
		}
		fmt.Printf("    %s %s\n", ui.Yellow("M"), f)
		shown++
	}
	for _, f := range status.UntrackedFiles {
		if shown >= maxShow {
			break
		}
		fmt.Printf("    %s %s\n", ui.Red("?"), f)
		shown++
	}

	if status.UncommittedCount > maxShow {
		fmt.Printf("    %s\n", ui.Dim(fmt.Sprintf("... and %d more", status.UncommittedCount-maxShow)))
	}
}
