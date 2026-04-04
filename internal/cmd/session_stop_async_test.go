package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniil-lyalko/clade/internal/session"
)

func TestSessionStopAsync_UpdatesSession(t *testing.T) {
	tmpDir := t.TempDir()
	reg := session.NewRegistry(tmpDir)
	inbox := session.NewInbox(tmpDir)

	// Create session in "stopping" state
	sess := &session.Session{
		SessionID:  "async-test",
		Project:    "proj",
		CWD:        "/tmp",
		Started:    time.Now().Add(-1 * time.Hour),
		LastActive: time.Now(),
		Status:     session.StatusStopping,
	}
	require.NoError(t, reg.Save(sess))

	// Create a transcript with some content
	transcriptDir := t.TempDir()
	transcriptPath := filepath.Join(transcriptDir, "transcript.jsonl")
	writeMinimalTranscript(t, transcriptPath, 5, true, true)

	input := &stopHookInput{
		SessionID:      "async-test",
		TranscriptPath: transcriptPath,
		CWD:            "/tmp",
	}

	err := doSessionStopAsync(reg, inbox, input)
	require.NoError(t, err)

	// Verify session is now stopped
	loaded, err := reg.Get("async-test")
	require.NoError(t, err)
	assert.Equal(t, session.StatusStopped, loaded.Status)
}

func TestSessionStopAsync_WritesInboxFYI(t *testing.T) {
	tmpDir := t.TempDir()
	reg := session.NewRegistry(tmpDir)
	inbox := session.NewInbox(tmpDir)

	sess := &session.Session{
		SessionID:  "inbox-test",
		Project:    "my-proj",
		CWD:        "/tmp",
		Started:    time.Now().Add(-1 * time.Hour),
		LastActive: time.Now(),
		Status:     session.StatusStopping,
	}
	require.NoError(t, reg.Save(sess))

	// Create transcript with assistant text
	transcriptDir := t.TempDir()
	transcriptPath := filepath.Join(transcriptDir, "transcript.jsonl")
	f, err := os.Create(transcriptPath)
	require.NoError(t, err)
	entry := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": "I fixed the Lambda timeout issue and pushed the changes."},
			},
		},
	}
	data, _ := json.Marshal(entry)
	f.Write(data)
	f.WriteString("\n")
	f.Close()

	input := &stopHookInput{
		SessionID:      "inbox-test",
		TranscriptPath: transcriptPath,
		CWD:            "/tmp",
	}

	err = doSessionStopAsync(reg, inbox, input)
	require.NoError(t, err)

	// Check inbox has an FYI entry
	entries, _, err := inbox.ReadRecent(0)
	require.NoError(t, err)
	found := false
	for _, e := range entries {
		if e.Project == "my-proj" && e.EntryType == session.EntryFYI {
			found = true
			break
		}
	}
	assert.True(t, found, "expected FYI inbox entry for my-proj")
}
