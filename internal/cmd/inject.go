package cmd

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/context"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/spf13/cobra"
)

var injectJSONFlag bool

var injectCmd = &cobra.Command{
	Use:    "inject-context",
	Short:  "Output session context (called by hooks)",
	Hidden: true, // Not meant to be called directly by users
	Long: `Outputs session context to stdout for the AI agent to consume.

This command is called automatically by hooks configured by 'clade init':
  - Claude Code: SessionStart hook (plain text output)
  - Cursor: sessionStart hook (JSON output, auto-detected)

The output format is auto-detected based on the calling agent:
  - Claude Code sets CLAUDE_PROJECT_DIR env var → plain text to stdout
  - Cursor does not set CLAUDE_PROJECT_DIR → JSON with additional_context
  - Terminal (manual test) → plain text to stdout

When Cursor loads both .cursor/hooks.json and .claude/settings.json (via
"Third-party hooks"), deduplication prevents double injection: the second
call within 3 seconds outputs nothing.

Use 'clade inject-context' from a terminal to test plain text output.
Use 'echo {} | clade inject-context --json' to test JSON output.`,
	RunE: runInjectContext,
}

func init() {
	rootCmd.AddCommand(injectCmd)
	// --json is deprecated; format is auto-detected now. Kept as hidden no-op
	// for backwards compatibility with existing .cursor/hooks.json files.
	injectCmd.Flags().BoolVar(&injectJSONFlag, "json", false, "Output as JSON (deprecated: auto-detected)")
	injectCmd.Flags().MarkHidden("json")
}

func runInjectContext(cmd *cobra.Command, args []string) error {
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Try to get repo root, fall back to cwd
	dir := cwd
	if git.IsGitRepo(cwd) {
		if root, err := git.GetRepoRoot(cwd); err == nil {
			dir = root
		}
	}

	// Dedup: if called twice within 3s for the same directory, skip.
	// This prevents double injection when Cursor fires both its own
	// .cursor/hooks.json and Claude's .claude/settings.json hooks.
	if wasRecentlyInjected(dir) {
		if needsJSONOutput() {
			// Cursor expects valid JSON even for empty responses
			return json.NewEncoder(os.Stdout).Encode(map[string]string{"additional_context": ""})
		}
		return nil
	}
	markInjected(dir)

	// Gather context
	ctx, err := context.GatherContext(dir)
	if err != nil {
		return fmt.Errorf("failed to gather context: %w", err)
	}

	// Format context
	output := context.FormatContext(ctx)

	// Check for recent hook failures
	if hookWarnings := checkHookFailures(dir); hookWarnings != "" {
		// Prepend hook warnings to context
		output = hookWarnings + "\n\n" + output
	}

	// Auto-detect output format:
	// - Claude Code sets CLAUDE_PROJECT_DIR → plain text to stdout
	// - Cursor does not set CLAUDE_PROJECT_DIR → JSON with additional_context
	// - The explicit --json flag also triggers JSON mode for backwards compat.
	if needsJSONOutput() {
		result := map[string]string{
			"additional_context": output,
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	// Claude Code (and terminal) format: plain text to stdout
	fmt.Print(output)
	return nil
}

// needsJSONOutput determines whether to output JSON (Cursor) or plain text (Claude Code).
//
// Claude Code sets CLAUDE_PROJECT_DIR when spawning hook commands, so its
// presence reliably identifies Claude Code. Both Claude Code and Cursor pipe
// JSON to stdin, so stdin pipe detection alone cannot distinguish them.
//
// The --json flag is kept for backwards compatibility with existing Cursor
// hook configs that explicitly pass it.
func needsJSONOutput() bool {
	if injectJSONFlag {
		return true
	}
	// Claude Code always sets CLAUDE_PROJECT_DIR for hook commands.
	if os.Getenv("CLAUDE_PROJECT_DIR") != "" {
		return false
	}
	// No CLAUDE_PROJECT_DIR and stdin is piped → likely Cursor.
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// dedupPath returns a temp file path unique to the given directory,
// used to coordinate between rapid successive invocations.
func dedupPath(dir string) string {
	h := fnv.New32a()
	h.Write([]byte(dir))
	return filepath.Join(os.TempDir(), fmt.Sprintf("clade-inject-%x", h.Sum32()))
}

// wasRecentlyInjected checks if inject-context was called for this
// directory within the last 3 seconds.
func wasRecentlyInjected(dir string) bool {
	info, err := os.Stat(dedupPath(dir))
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < 3*time.Second
}

// markInjected creates/touches the dedup marker file for this directory.
func markInjected(dir string) {
	// Security: Use restrictive permissions (owner-only)
	os.WriteFile(dedupPath(dir), []byte("1"), 0600)
}

// checkHookFailures reads .clade/last-hook-results.json and returns warnings for failed hooks.
func checkHookFailures(dir string) string {
	resultsPath := filepath.Join(dir, ".clade", "last-hook-results.json")

	data, err := os.ReadFile(resultsPath)
	if err != nil {
		return "" // No results file or can't read - skip silently
	}

	type jsonResult struct {
		Command  string  `json:"command"`
		Output   string  `json:"output,omitempty"`
		Error    string  `json:"error,omitempty"`
		Duration float64 `json:"duration_seconds"`
	}

	var results []jsonResult
	if err := json.Unmarshal(data, &results); err != nil {
		return ""
	}

	// Check if results are recent (within last 5 minutes)
	info, err := os.Stat(resultsPath)
	if err != nil || time.Since(info.ModTime()) > 5*time.Minute {
		return "" // Stale results, ignore
	}

	// Build warning message for failed hooks
	var warnings []string
	for _, r := range results {
		if r.Error != "" {
			warnings = append(warnings, fmt.Sprintf("- %s: %s", r.Command, r.Error))
		}
	}

	if len(warnings) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("## Hook Execution Warnings\n\n")
	buf.WriteString("The following lifecycle hooks failed:\n\n")
	for _, w := range warnings {
		buf.WriteString(w + "\n")
	}
	buf.WriteString("\nYou may need to run these commands manually.\n")

	return buf.String()
}
