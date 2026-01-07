package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// CreateWorktree creates a new git worktree with the given branch
func CreateWorktree(repoPath, worktreePath, branch string) error {
	// Fetch latest from origin
	fetchCmd := exec.Command("git", "fetch", "origin")
	fetchCmd.Dir = repoPath
	fetchCmd.Run() // Ignore errors - might be offline

	// Check if branch already exists
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = repoPath
	branchExists := cmd.Run() == nil

	var args []string
	if branchExists {
		// Branch exists, just checkout
		args = []string{"worktree", "add", worktreePath, branch}
	} else {
		// Create new branch from origin's default branch
		defaultBranch := GetDefaultBranch(repoPath)
		args = []string{"worktree", "add", "-b", branch, worktreePath, "origin/" + defaultBranch}
	}

	cmd = exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}

	return nil
}

// GetDefaultBranch returns the default branch (main, master, etc.) for a repo
func GetDefaultBranch(repoPath string) string {
	// Try to get from origin/HEAD
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err == nil {
		// Output is like "refs/remotes/origin/main"
		ref := strings.TrimSpace(string(output))
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: check if main exists, otherwise master
	cmd = exec.Command("git", "rev-parse", "--verify", "origin/main")
	cmd.Dir = repoPath
	if cmd.Run() == nil {
		return "main"
	}

	return "master"
}

// RemoveWorktree removes a git worktree
func RemoveWorktree(repoPath, worktreePath string) error {
	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %s: %w", string(output), err)
	}
	return nil
}

// ListWorktrees returns all worktrees for a repository
func ListWorktrees(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var worktrees []string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			worktrees = append(worktrees, strings.TrimPrefix(line, "worktree "))
		}
	}

	return worktrees, nil
}

// DeleteBranch deletes a git branch
func DeleteBranch(repoPath, branch string) error {
	cmd := exec.Command("git", "branch", "-D", branch)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete branch: %s: %w", string(output), err)
	}
	return nil
}

// GetCurrentBranch returns the current branch name
func GetCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetRepoRoot returns the root directory of the git repository
func GetRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// IsGitRepo checks if a path is inside a git repository
func IsGitRepo(path string) bool {
	_, err := GetRepoRoot(path)
	return err == nil
}

// GetRepoName returns the name of the repository (directory name)
func GetRepoName(repoPath string) string {
	return filepath.Base(repoPath)
}
