package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/daniil-lyalko/clade/internal/transcript"
	"github.com/spf13/cobra"
)

var sessionStopAsyncCmd = &cobra.Command{
	Use:    "session-stop-async",
	Short:  "Background async stop processing (called by session-stop)",
	Hidden: true,
	RunE:   runSessionStopAsync,
}

func init() {
	rootCmd.AddCommand(sessionStopAsyncCmd)
}

func runSessionStopAsync(cmd *cobra.Command, args []string) error {
	// Read input from CLADE_STOP_INPUT env var (stdin not available in background)
	inputJSON := os.Getenv("CLADE_STOP_INPUT")
	if inputJSON == "" {
		return nil
	}

	var input stopHookInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)
	inbox := session.NewInbox(baseDir)

	return doSessionStopAsync(reg, inbox, &input)
}

// doSessionStopAsync performs the full transcript parse, writes dropbag + inbox FYI,
// and marks the session as stopped. This runs as a background process.
func doSessionStopAsync(reg *session.Registry, inbox *session.Inbox, input *stopHookInput) error {
	sess, err := reg.Get(input.SessionID)
	if err != nil {
		// Session was deleted/archived while we were backgrounded
		return nil
	}

	var summary string

	// 1. Parse transcript
	if input.TranscriptPath != "" {
		extract, err := transcript.Parse(input.TranscriptPath)
		if err == nil && extract.TotalToolUses > 0 {
			// 2. Generate dropbag content
			content := transcript.FormatMarkdown(extract)

			// 3. Write session dropbag to ~/.clade/sessions/{session_id}.md
			dropbagPath := reg.DropbagPath(input.SessionID)
			os.MkdirAll(filepath.Dir(dropbagPath), 0755)
			os.WriteFile(dropbagPath, []byte(content), 0644)

			// Build summary from extract
			if extract.UserIntent != "" {
				summary = extract.UserIntent
				if len(summary) > 200 {
					summary = summary[:200] + "..."
				}
			}
		}
	}

	// 4. Append FYI entry to today's inbox (safety net for sessions that didn't broadcast)
	if summary == "" {
		summary = extractQuickSummary(input.TranscriptPath)
	}
	if summary != "" {
		entry := &session.InboxEntry{
			Time:      time.Now(),
			Project:   sess.Project,
			EntryType: session.EntryFYI,
			Message:   summary,
			SessionID: input.SessionID,
		}
		inbox.Append(entry) // best-effort, ignore errors
	}

	// 5. Update session to stopped with final summary
	sess.Status = session.StatusStopped
	sess.LastActive = time.Now()
	if summary != "" {
		sess.Summary = summary
	}

	return reg.Save(sess)
}
