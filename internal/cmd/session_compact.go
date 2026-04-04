package cmd

import (
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/spf13/cobra"
)

var sessionCompactCmd = &cobra.Command{
	Use:    "session-compact",
	Short:  "Update session on compaction (called by PreCompact hook)",
	Hidden: true,
	RunE:   runSessionCompact,
}

func init() {
	rootCmd.AddCommand(sessionCompactCmd)
}

func runSessionCompact(cmd *cobra.Command, args []string) error {
	input, err := readStopHookInput()
	if err != nil {
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)

	return doSessionCompact(reg, input)
}

// doSessionCompact updates the session's summary and token count on compaction.
func doSessionCompact(reg *session.Registry, input *stopHookInput) error {
	sess, err := reg.Get(input.SessionID)
	if err != nil {
		// Session not registered, create minimal
		sess = &session.Session{
			SessionID: input.SessionID,
			CWD:       input.CWD,
			Project:   detectProjectName(input.CWD),
			Started:   time.Now(),
			Status:    session.StatusActive,
		}
	}

	// Update last active time
	sess.LastActive = time.Now()

	// Try to grab a quick summary from the transcript tail
	if input.TranscriptPath != "" {
		if summary := extractQuickSummary(input.TranscriptPath); summary != "" {
			sess.Summary = summary
		}
	}

	return reg.Save(sess)
}
