package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/spf13/cobra"
)

var sessionStartCmd = &cobra.Command{
	Use:    "session-start",
	Short:  "Register session in the Clade session registry (called by SessionStart hook)",
	Hidden: true,
	RunE:   runSessionStart,
}

func init() {
	rootCmd.AddCommand(sessionStartCmd)
}

func runSessionStart(cmd *cobra.Command, args []string) error {
	input, err := readStopHookInput()
	if err != nil {
		// No stdin (manual invocation) — exit silently
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)

	return doSessionStart(reg, input)
}

// doSessionStart creates or updates a session entry in the registry.
// Extracted for testability.
func doSessionStart(reg *session.Registry, input *stopHookInput) error {
	// Skip subagent sessions — they're internal workers, not user sessions.
	// Claude Code sets CLAUDE_AGENT_ID for subagent processes (Agent tool).
	if os.Getenv("CLAUDE_AGENT_ID") != "" {
		return nil
	}

	cwd := input.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Skip if another active session already owns this CWD (likely a subagent
	// that inherited the parent's working directory).
	if sessions, err := reg.List(); err == nil {
		for _, s := range sessions {
			if s.Status == session.StatusActive && s.CWD == cwd && s.SessionID != input.SessionID {
				return nil
			}
		}
	}

	project := detectProjectName(cwd)
	branch := detectBranch(cwd)

	// Check if session already exists (resume case)
	existing, err := reg.Get(input.SessionID)
	if err == nil {
		// Resuming — update status and last_active
		existing.Status = session.StatusActive
		existing.LastActive = time.Now()
		if branch != "" {
			existing.Branch = branch
		}
		return reg.Save(existing)
	}

	// New session
	sess := &session.Session{
		SessionID:  input.SessionID,
		Project:    project,
		CWD:        cwd,
		Branch:     branch,
		Started:    time.Now(),
		LastActive: time.Now(),
		Status:     session.StatusActive,
	}

	// Detect if inside a clade worktree
	sess.IsWorktree, sess.WorktreeName = detectWorktree(cwd)

	return reg.Save(sess)
}

// detectProjectName extracts the project name from CWD.
// Uses the git remote origin name if available, otherwise the directory name.
func detectProjectName(cwd string) string {
	// Try git remote
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = cwd
	if out, err := cmd.Output(); err == nil {
		url := strings.TrimSpace(string(out))
		// Extract repo name from URL (handles both HTTPS and SSH)
		name := filepath.Base(url)
		name = strings.TrimSuffix(name, ".git")
		if name != "" && name != "." {
			return name
		}
	}

	return filepath.Base(cwd)
}

// detectBranch returns the current git branch for the directory.
func detectBranch(cwd string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = cwd
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// detectWorktree checks if the CWD is inside a clade-managed worktree.
// Returns (isWorktree, worktreeName).
func detectWorktree(cwd string) (bool, string) {
	// Check if inside a git worktree (not the main working tree)
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = cwd
	commonDir, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	cmd2 := exec.Command("git", "rev-parse", "--git-dir")
	cmd2.Dir = cwd
	gitDir, err := cmd2.Output()
	if err != nil {
		return false, ""
	}

	// If git-dir != git-common-dir, we're in a worktree
	if strings.TrimSpace(string(commonDir)) != strings.TrimSpace(string(gitDir)) {
		return true, filepath.Base(cwd)
	}

	return false, ""
}

// cladeBaseDir returns the base directory for Clade data (~/.clade/).
func cladeBaseDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".clade")
	}
	return filepath.Join(homeDir, ".clade")
}
