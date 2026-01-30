package git

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// validBranchPattern matches safe branch names:
// - Starts with alphanumeric
// - Contains only alphanumeric, forward slash, underscore, hyphen, or dot
// - No consecutive dots (..)
var validBranchPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/_.-]*$`)

// ValidateBranchName validates a branch name for safety
// Prevents command injection and path traversal attacks
func ValidateBranchName(branch string) error {
	if branch == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	if !validBranchPattern.MatchString(branch) {
		return fmt.Errorf("invalid branch name %q: use alphanumeric, /, _, -, . only (must start with alphanumeric)", branch)
	}

	// Prevent path traversal
	if strings.Contains(branch, "..") {
		return fmt.Errorf("invalid branch name %q: cannot contain '..'", branch)
	}

	// Prevent absolute paths
	if strings.HasPrefix(branch, "/") {
		return fmt.Errorf("invalid branch name %q: cannot start with '/'", branch)
	}

	// Prevent control characters and shell metacharacters
	for _, r := range branch {
		if r < 32 || r == 127 || r == ';' || r == '&' || r == '|' || r == '`' || r == '$' || r == '!' || r == '*' || r == '?' {
			return fmt.Errorf("invalid branch name %q: contains unsafe character %q", branch, r)
		}
	}

	return nil
}

// BranchStatus represents where a branch exists
type BranchStatus int

const (
	BranchNotFound BranchStatus = iota
	BranchLocalOnly
	BranchRemoteOnly
	BranchBoth
)

// BranchInfo contains information about a branch's status
type BranchInfo struct {
	Status       BranchStatus
	LocalAhead   int // commits ahead of remote
	RemoteBehind int // commits behind remote (remote is ahead)
	Diverged     bool
}

// CheckBranch checks if a branch exists locally and/or on remote
func CheckBranch(repoPath, branch string) BranchInfo {
	info := BranchInfo{Status: BranchNotFound}

	// Validate branch name before any git operations
	if err := ValidateBranchName(branch); err != nil {
		return info // Return NotFound for invalid branch names
	}

	localExists := branchExistsLocal(repoPath, branch)
	remoteExists := branchExistsRemote(repoPath, branch)

	if localExists && remoteExists {
		info.Status = BranchBoth
		info.LocalAhead, info.RemoteBehind, info.Diverged = getBranchDivergence(repoPath, branch)
	} else if localExists {
		info.Status = BranchLocalOnly
	} else if remoteExists {
		info.Status = BranchRemoteOnly
	}

	return info
}

// branchExistsLocal checks if branch exists locally
func branchExistsLocal(repoPath, branch string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

// branchExistsRemote checks if branch exists on origin
func branchExistsRemote(repoPath, branch string) bool {
	cmd := exec.Command("git", "ls-remote", "--heads", "origin", branch)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// getBranchDivergence returns local ahead, remote ahead, and whether diverged
func getBranchDivergence(repoPath, branch string) (localAhead, remoteAhead int, diverged bool) {
	cmd := exec.Command("git", "rev-list", "--left-right", "--count", branch+"...origin/"+branch)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, false
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) == 2 {
		localAhead, _ = strconv.Atoi(parts[0])
		remoteAhead, _ = strconv.Atoi(parts[1])
		diverged = localAhead > 0 && remoteAhead > 0
	}
	return
}

// Fetch fetches from origin
func Fetch(repoPath string) error {
	cmd := exec.Command("git", "fetch", "origin")
	cmd.Dir = repoPath
	return cmd.Run()
}

// CreateWorktreeNew creates a new worktree with a new branch.
// If baseBranch is empty, uses origin's default branch (main/master).
// Returns the actual base branch used and any error.
func CreateWorktreeNew(repoPath, worktreePath, branch, baseBranch string) (string, error) {
	// Validate branch name before any git operations
	if err := ValidateBranchName(branch); err != nil {
		return "", err
	}

	// Fetch first
	Fetch(repoPath) // Ignore error - might be offline

	// Check if branch exists anywhere
	info := CheckBranch(repoPath, branch)
	if info.Status != BranchNotFound {
		return "", fmt.Errorf("branch '%s' already exists", branch)
	}

	// Check if remote exists
	hasRemote := hasOriginRemote(repoPath)

	var cmd *exec.Cmd
	var actualBase string

	if baseBranch != "" {
		// User specified a base branch
		// Try origin/baseBranch first, fall back to local baseBranch
		if hasRemote {
			checkCmd := exec.Command("git", "rev-parse", "--verify", "origin/"+baseBranch)
			checkCmd.Dir = repoPath
			if checkCmd.Run() == nil {
				actualBase = "origin/" + baseBranch
			} else {
				// Try local branch
				checkCmd = exec.Command("git", "rev-parse", "--verify", baseBranch)
				checkCmd.Dir = repoPath
				if checkCmd.Run() == nil {
					actualBase = baseBranch
				} else {
					return "", fmt.Errorf("base branch '%s' not found (checked origin/%s and %s)", baseBranch, baseBranch, baseBranch)
				}
			}
		} else {
			// No remote - use local branch
			checkCmd := exec.Command("git", "rev-parse", "--verify", baseBranch)
			checkCmd.Dir = repoPath
			if checkCmd.Run() != nil {
				return "", fmt.Errorf("base branch '%s' not found", baseBranch)
			}
			actualBase = baseBranch
		}
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, actualBase)
	} else if hasRemote {
		// Default: create from origin's default branch
		defaultBranch := GetDefaultBranch(repoPath)
		actualBase = "origin/" + defaultBranch
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, actualBase)
	} else {
		// No remote - create from HEAD
		actualBase = "HEAD"
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, "HEAD")
	}

	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}

	return actualBase, nil
}

// hasOriginRemote checks if the repo has an origin remote configured
func hasOriginRemote(repoPath string) bool {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

// CreateWorktreeFromBranch creates a worktree from an existing local branch
func CreateWorktreeFromBranch(repoPath, worktreePath, branch string) error {
	// Validate branch name before any git operations
	if err := ValidateBranchName(branch); err != nil {
		return err
	}

	cmd := exec.Command("git", "worktree", "add", worktreePath, branch)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}
	return nil
}

// CreateWorktreeTrackRemote creates a worktree tracking a remote branch
func CreateWorktreeTrackRemote(repoPath, worktreePath, branch string) error {
	// Validate branch name before any git operations
	if err := ValidateBranchName(branch); err != nil {
		return err
	}

	// Create local branch tracking remote
	cmd := exec.Command("git", "worktree", "add", "--track", "-b", branch, worktreePath, "origin/"+branch)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}
	return nil
}

// PreflightCheck checks branch status for multiple repos
// Returns a map of repo path -> BranchInfo
func PreflightCheck(repos []string, branch string) map[string]BranchInfo {
	results := make(map[string]BranchInfo)
	for _, repo := range repos {
		Fetch(repo) // Fetch each repo first
		results[repo] = CheckBranch(repo, branch)
	}
	return results
}
