package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/clade/internal/git"
)

// CladeMetadata represents the .clade.json file
type CladeMetadata struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Ticket  string `json:"ticket,omitempty"`
	Repo    string `json:"repo"`
	Created string `json:"created"`
}

// ContextOutput holds all the context to be injected
type ContextOutput struct {
	Dropbag    *DropbagInfo
	GitStatus  *git.Status
	Commits    []string
	Todos      []TodoItem
	Metadata   *CladeMetadata
	RepoName   string
	BranchName string
}

// GatherContext collects all context information for a directory
func GatherContext(dir string) (*ContextOutput, error) {
	ctx := &ContextOutput{}

	// Get repo name and branch
	ctx.RepoName = git.GetRepoName(dir)
	if branch, err := git.GetCurrentBranch(dir); err == nil {
		ctx.BranchName = branch
	}

	// Read DROPBAG.md
	if dropbag, err := ReadDropbag(dir); err == nil {
		ctx.Dropbag = dropbag
	}

	// Get git status
	if status, err := git.GetStatus(dir); err == nil {
		ctx.GitStatus = status
	}

	// Get recent commits
	if commits, err := git.GetRecentCommits(dir, 5); err == nil {
		ctx.Commits = commits
	}

	// Find TODOs
	if todos, err := FindTodos(dir, 10); err == nil {
		ctx.Todos = todos
	}

	// Read .clade.json metadata
	metadata, _ := ReadCladeMetadata(dir)
	ctx.Metadata = metadata

	return ctx, nil
}

// FormatContext formats the context for output to Claude
func FormatContext(ctx *ContextOutput) string {
	var sb strings.Builder

	sb.WriteString("# Session Context\n\n")

	// DROPBAG.md section
	if ctx.Dropbag != nil && ctx.Dropbag.Exists {
		sb.WriteString(fmt.Sprintf("## DROPBAG.md (from %s)\n\n", ctx.Dropbag.RelativeAge))
		sb.WriteString(ctx.Dropbag.Content)
		sb.WriteString("\n\n")
	}

	// Git Status section
	if ctx.GitStatus != nil {
		sb.WriteString("## Git Status\n\n")
		sb.WriteString(fmt.Sprintf("On branch %s\n", ctx.BranchName))

		if ctx.GitStatus.Clean {
			sb.WriteString("Working tree clean\n")
		} else {
			if len(ctx.GitStatus.StagedFiles) > 0 {
				sb.WriteString("\nStaged changes:\n")
				for _, f := range ctx.GitStatus.StagedFiles {
					sb.WriteString(fmt.Sprintf("  %s\n", f))
				}
			}
			if len(ctx.GitStatus.ModifiedFiles) > 0 {
				sb.WriteString("\nModified files:\n")
				for _, f := range ctx.GitStatus.ModifiedFiles {
					sb.WriteString(fmt.Sprintf("  modified: %s\n", f))
				}
			}
			if len(ctx.GitStatus.UntrackedFiles) > 0 {
				sb.WriteString("\nUntracked files:\n")
				for _, f := range ctx.GitStatus.UntrackedFiles {
					sb.WriteString(fmt.Sprintf("  %s\n", f))
				}
			}
		}
		sb.WriteString("\n")
	}

	// Recent Commits section
	if len(ctx.Commits) > 0 {
		sb.WriteString("## Recent Commits\n\n")
		for _, commit := range ctx.Commits {
			sb.WriteString(fmt.Sprintf("%s\n", commit))
		}
		sb.WriteString("\n")
	}

	// Open TODOs section
	if len(ctx.Todos) > 0 {
		sb.WriteString("## Open TODOs\n\n")
		for _, todo := range ctx.Todos {
			sb.WriteString(fmt.Sprintf("%s:%d: %s\n", todo.File, todo.Line, todo.Content))
		}
		sb.WriteString("\n")
	}

	// Ticket section
	if ctx.Metadata != nil && ctx.Metadata.Ticket != "" {
		sb.WriteString("## Ticket\n\n")
		sb.WriteString(fmt.Sprintf("%s detected. ", ctx.Metadata.Ticket))

		// Check if TICKET.md exists
		ticketPath := filepath.Join(".", "TICKET.md")
		if _, err := os.Stat(ticketPath); os.IsNotExist(err) {
			sb.WriteString("Please fetch from JIRA and save to TICKET.md for reference.\n")
		} else {
			sb.WriteString("See TICKET.md for details.\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ReadCladeMetadata reads the .clade.json file from a directory
func ReadCladeMetadata(dir string) (*CladeMetadata, error) {
	path := filepath.Join(dir, ".clade.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var metadata CladeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}
