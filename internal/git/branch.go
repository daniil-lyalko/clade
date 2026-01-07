package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

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

// CreateWorktreeNew creates a new worktree with a new branch from origin's default
// Returns error if branch already exists anywhere
func CreateWorktreeNew(repoPath, worktreePath, branch string) error {
	// Fetch first
	Fetch(repoPath) // Ignore error - might be offline

	// Check if branch exists anywhere
	info := CheckBranch(repoPath, branch)
	if info.Status != BranchNotFound {
		return fmt.Errorf("branch '%s' already exists", branch)
	}

	// Check if remote exists
	hasRemote := hasOriginRemote(repoPath)

	var cmd *exec.Cmd
	if hasRemote {
		// Create new branch from origin's default branch
		defaultBranch := GetDefaultBranch(repoPath)
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, "origin/"+defaultBranch)
	} else {
		// No remote - create from HEAD
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, "HEAD")
	}

	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}

	return nil
}

// hasOriginRemote checks if the repo has an origin remote configured
func hasOriginRemote(repoPath string) bool {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

// CreateWorktreeFromBranch creates a worktree from an existing local branch
func CreateWorktreeFromBranch(repoPath, worktreePath, branch string) error {
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
