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

// WorktreeInfo contains detailed information about a git worktree
type WorktreeInfo struct {
	Path   string // Absolute path to worktree
	Branch string // Branch name (without refs/heads/)
	Head   string // Commit SHA
	IsMain bool   // True if this is the main repo, not a worktree
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

// ListWorktreesDetailed returns detailed info about all worktrees for a repository
func ListWorktreesDetailed(repoPath string) ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Get main repo path to identify which entry is the main worktree
	mainRepoPath, _ := GetMainRepoRoot(repoPath)

	var worktrees []WorktreeInfo
	var current WorktreeInfo

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line marks end of a worktree entry
			if current.Path != "" {
				// Check if this is the main repo
				current.IsMain = (current.Path == mainRepoPath)
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Head = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			// Branch is like "refs/heads/main" - extract just the branch name
			branch := strings.TrimPrefix(line, "branch ")
			branch = strings.TrimPrefix(branch, "refs/heads/")
			current.Branch = branch
		} else if line == "detached" {
			current.Branch = "(detached)"
		}
	}

	// Don't forget the last entry if output doesn't end with newline
	if current.Path != "" {
		current.IsMain = (current.Path == mainRepoPath)
		worktrees = append(worktrees, current)
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

// GetRepoRoot returns the root directory of the git repository (worktree-aware)
// If in a worktree, returns the worktree's root, not the main repo
func GetRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetMainRepoRoot returns the root directory of the main repository
// If in a worktree, resolves to the original/main repo, not the worktree
func GetMainRepoRoot(path string) (string, error) {
	// Get the common git directory (shared across all worktrees)
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	gitCommonDir := strings.TrimSpace(string(output))

	// If it's just ".git", we're in the main repo
	if gitCommonDir == ".git" {
		return GetRepoRoot(path)
	}

	// Otherwise, gitCommonDir is an absolute path like /path/to/main-repo/.git
	// The main repo root is the parent of .git
	if strings.HasSuffix(gitCommonDir, "/.git") || strings.HasSuffix(gitCommonDir, "\\.git") {
		return filepath.Dir(gitCommonDir), nil
	}

	// Fallback: resolve to absolute and get parent
	absGitDir, err := filepath.Abs(filepath.Join(path, gitCommonDir))
	if err != nil {
		return GetRepoRoot(path)
	}

	// gitCommonDir might be like "../../../main-repo/.git"
	if strings.HasSuffix(absGitDir, "/.git") || strings.HasSuffix(absGitDir, "\\.git") {
		return filepath.Dir(absGitDir), nil
	}

	// Last fallback
	return GetRepoRoot(path)
}

// IsWorktree checks if the current path is inside a git worktree (not the main repo)
func IsWorktree(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	gitCommonDir := strings.TrimSpace(string(output))
	return gitCommonDir != ".git"
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

// PruneWorktrees removes stale worktree entries where the directory no longer exists
func PruneWorktrees(repoPath string) error {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// HasPrunableWorktrees checks if there are stale worktree entries that can be pruned
func HasPrunableWorktrees(repoPath string) (bool, error) {
	cmd := exec.Command("git", "worktree", "prune", "--dry-run")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to check prunable worktrees: %w", err)
	}
	// Output is non-empty if there are prunable entries
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// RepairWorktrees repairs worktree links after directories have been moved/renamed.
// If worktreePaths are provided, repairs those specific paths.
// If no paths provided, attempts to repair all worktrees from the main repo.
func RepairWorktrees(repoPath string, worktreePaths ...string) error {
	args := []string{"worktree", "repair"}
	args = append(args, worktreePaths...)

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Repair can fail on individual paths but still fix others
		// Don't treat as fatal error, just log
		return fmt.Errorf("worktree repair: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
