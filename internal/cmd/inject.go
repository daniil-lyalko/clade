package cmd

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
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

The output format is auto-detected based on how the command is invoked:
  - If stdin is a pipe (Cursor), outputs JSON with additional_context field
  - If stdin is a terminal (Claude Code), outputs plain text

When Cursor loads both .cursor/hooks.json and .claude/settings.json (via
"Third-party hooks"), deduplication prevents double injection: the second
call within 3 seconds outputs nothing.

Use 'clade inject-context' from a terminal to test plain text output.
Use 'echo {} | clade inject-context' to test JSON output.`,
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
		if isCursorHook() {
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

	// Auto-detect output format:
	// - Cursor pipes JSON to stdin → output JSON with additional_context
	// - Claude Code attaches to TTY → output plain text
	// The explicit --json flag also triggers JSON mode for backwards compat.
	if isCursorHook() || injectJSONFlag {
		result := map[string]string{
			"additional_context": output,
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	// Claude Code format: plain text to stdout
	fmt.Print(output)
	return nil
}

// isCursorHook detects whether the command was invoked by Cursor.
// Cursor pipes JSON on stdin; Claude Code runs with stdin attached to a TTY.
func isCursorHook() bool {
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
	os.WriteFile(dedupPath(dir), []byte("1"), 0644)
}
