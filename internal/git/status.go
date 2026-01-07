package git

import (
	"os/exec"
	"strings"
)

// Status represents the git status of a repository
type Status struct {
	Clean             bool
	ModifiedFiles     []string
	UntrackedFiles    []string
	StagedFiles       []string
	UncommittedCount  int
}

// GetStatus returns the git status for a repository
func GetStatus(repoPath string) (*Status, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	status := &Status{
		Clean: true,
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		status.Clean = false
		status.UncommittedCount++

		if len(line) < 3 {
			continue
		}

		indexStatus := line[0]
		workTreeStatus := line[1]
		file := strings.TrimSpace(line[3:])

		// Staged files
		if indexStatus != ' ' && indexStatus != '?' {
			status.StagedFiles = append(status.StagedFiles, file)
		}

		// Modified in work tree
		if workTreeStatus == 'M' {
			status.ModifiedFiles = append(status.ModifiedFiles, file)
		}

		// Untracked
		if indexStatus == '?' {
			status.UntrackedFiles = append(status.UntrackedFiles, file)
		}
	}

	return status, nil
}

// HasUncommittedChanges returns true if there are uncommitted changes
func HasUncommittedChanges(repoPath string) (bool, error) {
	status, err := GetStatus(repoPath)
	if err != nil {
		return false, err
	}
	return !status.Clean, nil
}

// GetRecentCommits returns recent commit messages
func GetRecentCommits(repoPath string, count int) ([]string, error) {
	cmd := exec.Command("git", "log", "--oneline", "-n", string(rune('0'+count)))
	if count > 9 {
		cmd = exec.Command("git", "log", "--oneline", "-n", "10")
	}
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var commits []string
	for _, line := range lines {
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits, nil
}
