package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/transcript"
	"github.com/spf13/cobra"
)

var autoDROPBAGCmd = &cobra.Command{
	Use:    "auto-dropbag",
	Short:  "Auto-generate DROPBAG from session transcript (called by Stop/PreCompact hook)",
	Hidden: true,
	Long: `Parses the Claude Code session transcript (JSONL) to extract meaningful
session context, then writes a structured DROPBAG-auto.md file.

Called by Stop and PreCompact hooks. Reads hook input JSON from stdin
containing session_id, transcript_path, and cwd.

Pipeline:
  1. Read stdin JSON for transcript_path
  2. Parse JSONL transcript for files, commands, errors, intent
  3. Attempt CLI summarization (claude -p) with extracted data
  4. Fallback to mechanical markdown if CLI unavailable
  5. Write .clade/DROPBAG-auto.md`,
	RunE: runAutoDROPBAG,
}

func init() {
	rootCmd.AddCommand(autoDROPBAGCmd)
}

// stopHookInput is the JSON payload from Claude Code Stop/PreCompact hooks.
type stopHookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	StopHookActive bool   `json:"stop_hook_active"`
	HookEventName  string `json:"hook_event_name"`
}

// debounceState tracks when auto-dropbag last ran to avoid redundant writes.
type debounceState struct {
	TranscriptSize int64  `json:"transcript_size"`
	LastWrite      string `json:"last_write"` // RFC3339
}

func runAutoDROPBAG(cmd *cobra.Command, args []string) error {
	// 1. Read stdin JSON
	input, err := readStopHookInput()
	if err != nil {
		// If stdin is empty or invalid (manual invocation), exit silently
		return nil
	}

	// 2. Loop guard — prevent recursion if this hook triggers itself
	if input.StopHookActive {
		return nil
	}

	// 3. Validate transcript path
	if input.TranscriptPath == "" {
		return nil
	}
	if _, err := os.Stat(input.TranscriptPath); os.IsNotExist(err) {
		return nil
	}

	// Determine working directory
	cwd := input.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if cwd == "" {
		return nil
	}

	// 4. Debounce — skip if transcript size unchanged and <5min since last write
	if !shouldUpdate(cwd, input.TranscriptPath) {
		return nil
	}

	// 5. Parse transcript
	extract, err := transcript.Parse(input.TranscriptPath)
	if err != nil {
		return nil // silent failure
	}

	// Skip if transcript had no meaningful content
	if extract.TotalToolUses == 0 && extract.UserIntent == "" {
		return nil
	}

	// 6. Try CLI summarization, fallback to mechanical markdown
	content := generateDropbagContent(extract)

	// 7. Write .clade/DROPBAG-auto.md
	cladeDir := filepath.Join(cwd, ".clade")
	if err := os.MkdirAll(cladeDir, 0755); err != nil {
		return nil
	}

	outputPath := filepath.Join(cladeDir, "DROPBAG-auto.md")
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return nil
	}

	// Update debounce state
	saveDebounceState(cwd, input.TranscriptPath)

	return nil
}

// readStopHookInput reads the hook JSON payload from stdin.
func readStopHookInput() (*stopHookInput, error) {
	// Check if stdin has data (not a terminal)
	stat, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return nil, fmt.Errorf("no stdin data")
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty stdin")
	}

	var input stopHookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

// shouldUpdate returns true if auto-dropbag should run (debounce check).
// Time-gated: must be ≥5 minutes since last write AND transcript must have grown.
// This prevents expensive CLI summarization on every assistant turn (Stop fires per-turn).
func shouldUpdate(cwd, transcriptPath string) bool {
	statePath := debounceStatePath(cwd)

	data, err := os.ReadFile(statePath)
	if err != nil {
		return true // no previous state, first run
	}

	var state debounceState
	if err := json.Unmarshal(data, &state); err != nil {
		return true
	}

	// Time gate first — never run more than once per 5 minutes
	lastWrite, err := time.Parse(time.RFC3339, state.LastWrite)
	if err != nil {
		return true
	}
	if time.Since(lastWrite) < 5*time.Minute {
		return false // too recent, skip regardless of transcript growth
	}

	// Time gate passed — only run if transcript actually grew
	info, err := os.Stat(transcriptPath)
	if err != nil {
		return true
	}
	return info.Size() != state.TranscriptSize
}

// saveDebounceState records the current transcript size and timestamp.
func saveDebounceState(cwd, transcriptPath string) {
	info, err := os.Stat(transcriptPath)
	if err != nil {
		return
	}

	state := debounceState{
		TranscriptSize: info.Size(),
		LastWrite:      time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	statePath := debounceStatePath(cwd)
	os.WriteFile(statePath, data, 0644)
}

func debounceStatePath(cwd string) string {
	return filepath.Join(cwd, ".clade", ".dropbag-state.json")
}

// generateDropbagContent attempts CLI summarization and falls back to mechanical markdown.
func generateDropbagContent(extract *transcript.SessionExtract) string {
	// Try CLI summarization
	summary, err := cliSummarize(extract)
	if err == nil && summary != "" {
		return formatCLISummary(summary, extract)
	}

	// Fallback to mechanical markdown
	return transcript.FormatMarkdown(extract)
}

// cliSummarize runs a headless Claude/Cursor CLI to summarize the session extract.
func cliSummarize(extract *transcript.SessionExtract) (string, error) {
	cli, args := detectCLI()
	if cli == "" {
		return "", fmt.Errorf("no CLI available")
	}

	prompt := transcript.FormatPrompt(extract)

	// Build command: cli <args> -p "prompt"
	cmdArgs := append(args, "-p", prompt)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cli, cmdArgs...)
	cmd.Stdin = nil // no stdin needed
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return stdout.String(), nil
}

// detectCLI finds the best available CLI for summarization.
func detectCLI() (string, []string) {
	// Try config-based detection first
	cfg, err := config.Load()
	if err == nil {
		switch cfg.Agent {
		case "claude":
			if path, err := exec.LookPath("claude"); err == nil {
				return path, []string{"--output-format", "text"}
			}
		}
	}

	// Fallback chain: try common CLI tools
	if path, err := exec.LookPath("claude"); err == nil {
		return path, []string{"--output-format", "text"}
	}

	return "", nil
}

// formatCLISummary wraps CLI-generated summary with metadata.
func formatCLISummary(summary string, extract *transcript.SessionExtract) string {
	var buf bytes.Buffer

	buf.WriteString("# Session Context (AI-summarized)\n\n")
	buf.WriteString("_From previous session — verify current state before acting._\n\n")

	buf.WriteString(summary)

	// Append compact stats
	buf.WriteString("\n\n---\n")
	if extract.Branch != "" {
		fmt.Fprintf(&buf, "_Branch: %s", extract.Branch)
	}
	if extract.TotalToolUses > 0 {
		fmt.Fprintf(&buf, " | %d tool uses", extract.TotalToolUses)
	}
	if !extract.StartTime.IsZero() && !extract.EndTime.IsZero() {
		duration := extract.EndTime.Sub(extract.StartTime)
		if duration > 0 {
			fmt.Fprintf(&buf, " | duration: %s", formatDuration(duration))
		}
	}
	buf.WriteString("_\n")

	return buf.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}
