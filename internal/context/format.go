package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	Dropbag     *DropbagInfo
	AutoDropbag *DropbagInfo // Auto-generated from session transcript
	GitStatus   *git.Status
	Commits     []string
	Todos       []TodoItem
	Metadata    *CladeMetadata
	RepoName    string
	BranchName  string
	Dir         string // Directory where context was gathered (for path resolution)
}

// GatherContext collects all context information for a directory
func GatherContext(dir string) (*ContextOutput, error) {
	ctx := &ContextOutput{
		Dir: dir, // Store directory for path resolution in FormatContext
	}

	// Get repo name and branch
	ctx.RepoName = git.GetRepoName(dir)
	if branch, err := git.GetCurrentBranch(dir); err == nil {
		ctx.BranchName = branch
	}

	// Read DROPBAG.md (manual /drop)
	if dropbag, err := ReadDropbag(dir); err == nil {
		ctx.Dropbag = dropbag
	}

	// Read DROPBAG-auto.md (auto-generated from transcript)
	if autoDropbag, err := ReadAutoDropbag(dir); err == nil {
		ctx.AutoDropbag = autoDropbag
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

	// Auto-DROPBAG section (from previous session transcript, most recent context)
	if ctx.AutoDropbag != nil && ctx.AutoDropbag.Exists {
		header := fmt.Sprintf("## Session Context (from %s)", ctx.AutoDropbag.RelativeAge)

		age := time.Since(ctx.AutoDropbag.ModTime)
		isStale := age > 48*time.Hour

		if isStale {
			header += " ⚠️  STALE"
		}
		sb.WriteString(header + "\n\n")

		sb.WriteString(ctx.AutoDropbag.Content)
		sb.WriteString("\n\n")
	}

	// Manual DROPBAG section (from /drop command)
	if ctx.Dropbag != nil && ctx.Dropbag.Exists {
		header := fmt.Sprintf("## DROPBAG (from %s)", ctx.Dropbag.RelativeAge)

		// Add stale warning if >2 days old
		age := time.Since(ctx.Dropbag.ModTime)
		isStale := age > 48*time.Hour

		if isStale {
			header += " ⚠️  STALE"
		}
		sb.WriteString(header + "\n\n")

		// Prompt to update if stale
		if isStale {
			sb.WriteString("_Consider updating with /drop if context has changed since last session_\n\n")
		}

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

		// Check if TICKET.md exists in the worktree directory
		// Fixed: Use ctx.Dir instead of "." to resolve path correctly
		ticketPath := filepath.Join(ctx.Dir, "TICKET.md")
		if _, err := os.Stat(ticketPath); os.IsNotExist(err) {
			sb.WriteString("Please fetch from JIRA and save to TICKET.md for reference.\n")
		} else {
			sb.WriteString("See TICKET.md for details.\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ReadCladeMetadata reads the metadata file from a directory.
// Supports both new path (.clade/metadata.json) and legacy path (.clade.json).
// Auto-migrates from legacy to new path if found.
func ReadCladeMetadata(dir string) (*CladeMetadata, error) {
	newPath := filepath.Join(dir, ".clade", "metadata.json")
	oldPath := filepath.Join(dir, ".clade.json")

	// Try new path first
	if data, err := os.ReadFile(newPath); err == nil {
		var metadata CladeMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			return nil, err
		}
		return &metadata, nil
	}

	// Try legacy path and auto-migrate if found
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return nil, err
	}

	var metadata CladeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	// Auto-migrate: move to new location
	cladeDir := filepath.Join(dir, ".clade")
	if err := os.MkdirAll(cladeDir, 0755); err == nil {
		if err := os.Rename(oldPath, newPath); err == nil {
			// Migration successful - silently continue
		}
	}

	return &metadata, nil
}
