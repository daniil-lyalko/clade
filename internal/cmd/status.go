package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/daniil-lyalko/clade/internal/context"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var statusJSONFlag bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show context for current directory",
	Long: `Display clade context information for the current directory.

Shows:
  - Worktree/project metadata
  - Context files (CLAUDE.md, DROPBAG.md, TICKET.md)
  - Git status summary
  - Recent commits

Works in any directory:
  - In a clade worktree: Full context info
  - In a regular git repo: Basic info + suggestion to init
  - Not in git repo: Clear message

Use --json for machine-readable output.`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&statusJSONFlag, "json", false, "Output as JSON")
}

// StatusOutputJSON is the JSON output structure for status command
type StatusOutputJSON struct {
	InGitRepo    bool             `json:"in_git_repo"`
	IsCladeManaged bool           `json:"is_clade_managed"`
	RepoRoot     string           `json:"repo_root,omitempty"`
	Branch       string           `json:"branch,omitempty"`
	Name         string           `json:"name,omitempty"`
	Type         string           `json:"type,omitempty"`
	Repo         string           `json:"repo,omitempty"`
	Ticket       string           `json:"ticket,omitempty"`
	ContextFiles []StatusFileJSON `json:"context_files,omitempty"`
	HooksConfigured bool          `json:"hooks_configured"`
	GitStatus    *StatusGitJSON   `json:"git_status,omitempty"`
}

// StatusFileJSON represents a context file in JSON output
type StatusFileJSON struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
}

// StatusGitJSON represents git status in JSON output
type StatusGitJSON struct {
	Clean            bool     `json:"clean"`
	UncommittedCount int      `json:"uncommitted_count"`
	StagedFiles      []string `json:"staged_files,omitempty"`
	ModifiedFiles    []string `json:"modified_files,omitempty"`
	UntrackedFiles   []string `json:"untracked_files,omitempty"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Check if we're in a git repo
	if !git.IsGitRepo(cwd) {
		if statusJSONFlag {
			output := StatusOutputJSON{InGitRepo: false}
			data, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		ui.Info("Not in a git repository")
		ui.Detail("Navigate to a git repo or clade experiment")
		return nil
	}

	repoRoot, err := git.GetRepoRoot(cwd)
	if err != nil {
		return err
	}

	// Check for .clade.json (indicates clade-managed worktree)
	metadata, _ := context.ReadCladeMetadata(repoRoot)

	if statusJSONFlag {
		return printStatusJSON(repoRoot, metadata)
	}

	if metadata != nil {
		// We're in a clade-managed worktree
		printCladeStatus(repoRoot, metadata)
	} else {
		// Regular git repo, not clade-managed
		printBasicStatus(repoRoot)
	}

	return nil
}

func printStatusJSON(repoRoot string, metadata *context.CladeMetadata) error {
	output := StatusOutputJSON{
		InGitRepo: true,
		RepoRoot:  repoRoot,
	}

	// Branch
	if branch, err := git.GetCurrentBranch(repoRoot); err == nil {
		output.Branch = branch
	}

	// Check hooks
	claudeSettingsPath := filepath.Join(repoRoot, ".claude", "settings.json")
	_, err := os.Stat(claudeSettingsPath)
	output.HooksConfigured = err == nil

	if metadata != nil {
		output.IsCladeManaged = true
		output.Name = metadata.Name
		output.Type = metadata.Type
		output.Repo = metadata.Repo
		output.Ticket = metadata.Ticket

		// Context files
		contextFiles := []string{"CLAUDE.md", "DROPBAG.md"}
		if metadata.Ticket != "" {
			contextFiles = append(contextFiles, "TICKET.md")
		}
		for _, f := range contextFiles {
			path := filepath.Join(repoRoot, f)
			_, err := os.Stat(path)
			output.ContextFiles = append(output.ContextFiles, StatusFileJSON{
				Name:   f,
				Exists: err == nil,
			})
		}
	}

	// Git status
	status, err := git.GetStatus(repoRoot)
	if err == nil {
		output.GitStatus = &StatusGitJSON{
			Clean:            status.Clean,
			UncommittedCount: status.UncommittedCount,
			StagedFiles:      status.StagedFiles,
			ModifiedFiles:    status.ModifiedFiles,
			UntrackedFiles:   status.UntrackedFiles,
		}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printCladeStatus(repoRoot string, metadata *context.CladeMetadata) {
	// Header with type
	typeLabel := "Worktree"
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
	fmt.Printf("  %s\n", ui.Dim("(not a clade worktree)"))

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
	ui.Info("This is a regular git repo, not a clade worktree")
	ui.Detail("Create a worktree: clade <name>")
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
