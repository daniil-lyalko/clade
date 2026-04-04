package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniil-lyalko/clade/internal/session"
)

func TestTriageTier_Trivial(t *testing.T) {
	// No tool uses, < 3 user messages
	tier := triageSession(2, false, false, false)
	assert.Equal(t, tierTrivial, tier)
}

func TestTriageTier_Light(t *testing.T) {
	// Some activity but inbox entries were already written
	tier := triageSession(10, true, true, true)
	assert.Equal(t, tierLight, tier)
}

func TestTriageTier_NeedsAsync(t *testing.T) {
	// Edits happened but no inbox entries
	tier := triageSession(10, true, true, false)
	assert.Equal(t, tierAsync, tier)
}

func TestTriageTier_CommandsNoInbox(t *testing.T) {
	// Many commands but no inbox entries
	tier := triageSession(10, false, true, false)
	assert.Equal(t, tierAsync, tier)
}

func TestSessionStop_Trivial(t *testing.T) {
	tmpDir := t.TempDir()
	reg := session.NewRegistry(tmpDir)

	// Create an active session
	sess := &session.Session{
		SessionID: "trivial-sess",
		Project:   "test",
		CWD:       "/tmp",
		Status:    session.StatusActive,
	}
	require.NoError(t, reg.Save(sess))

	// Write a minimal transcript (just 2 user messages, no tool use)
	transcriptDir := t.TempDir()
	transcriptPath := filepath.Join(transcriptDir, "transcript.jsonl")
	writeMinimalTranscript(t, transcriptPath, 2, false, false)

	input := &stopHookInput{
		SessionID:      "trivial-sess",
		TranscriptPath: transcriptPath,
		CWD:            "/tmp",
	}

	err := doSessionStop(reg, session.NewInbox(tmpDir), input)
	require.NoError(t, err)

	loaded, err := reg.Get("trivial-sess")
	require.NoError(t, err)
	assert.Equal(t, session.StatusStopped, loaded.Status)
}

func TestQuickTranscriptScan_Empty(t *testing.T) {
	userMsgs, hasEdits, hasCommands := quickTranscriptScan("")
	assert.Equal(t, 0, userMsgs)
	assert.False(t, hasEdits)
	assert.False(t, hasCommands)
}

func TestQuickTranscriptScan_NonExistent(t *testing.T) {
	userMsgs, hasEdits, hasCommands := quickTranscriptScan("/nonexistent/path.jsonl")
	assert.Equal(t, 0, userMsgs)
	assert.False(t, hasEdits)
	assert.False(t, hasCommands)
}

func TestQuickTranscriptScan_UsersOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	writeMinimalTranscript(t, path, 5, false, false)

	userMsgs, hasEdits, hasCommands := quickTranscriptScan(path)
	assert.Equal(t, 5, userMsgs)
	assert.False(t, hasEdits)
	assert.False(t, hasCommands)
}

func TestQuickTranscriptScan_WithEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	writeMinimalTranscript(t, path, 3, true, false)

	userMsgs, hasEdits, hasCommands := quickTranscriptScan(path)
	assert.Equal(t, 3, userMsgs)
	assert.True(t, hasEdits)
	assert.False(t, hasCommands)
}

func TestQuickTranscriptScan_WithCommands(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	writeMinimalTranscript(t, path, 2, false, true)

	userMsgs, hasEdits, hasCommands := quickTranscriptScan(path)
	assert.Equal(t, 2, userMsgs)
	assert.False(t, hasEdits)
	assert.True(t, hasCommands) // >3 Bash calls
}

func TestExtractQuickSummary_Empty(t *testing.T) {
	assert.Equal(t, "", extractQuickSummary(""))
}

func TestExtractQuickSummary_NonExistent(t *testing.T) {
	assert.Equal(t, "", extractQuickSummary("/nonexistent/path.jsonl"))
}

func TestExtractQuickSummary_ReturnsLastAssistantText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	f, err := os.Create(path)
	require.NoError(t, err)

	// Write a user message
	user := map[string]interface{}{
		"type":    "user",
		"message": map[string]interface{}{"role": "user", "content": "fix the bug"},
	}
	data, _ := json.Marshal(user)
	f.Write(data)
	f.WriteString("\n")

	// Write an assistant message
	assistant := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": "I fixed the Lambda timeout issue."},
			},
		},
	}
	data, _ = json.Marshal(assistant)
	f.Write(data)
	f.WriteString("\n")
	f.Close()

	summary := extractQuickSummary(path)
	assert.Equal(t, "I fixed the Lambda timeout issue.", summary)
}

func TestExtractQuickSummary_Truncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	f, err := os.Create(path)
	require.NoError(t, err)

	longText := strings.Repeat("x", 400)
	assistant := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": longText},
			},
		},
	}
	data, _ := json.Marshal(assistant)
	f.Write(data)
	f.WriteString("\n")
	f.Close()

	summary := extractQuickSummary(path)
	assert.LessOrEqual(t, len(summary), 304) // 300 + "..."
	assert.True(t, strings.HasSuffix(summary, "..."))
}

func TestSessionHasInboxEntries_NoFiles(t *testing.T) {
	dir := t.TempDir()
	inbox := session.NewInbox(dir)
	sess := &session.Session{Project: "my-project"}
	assert.False(t, sessionHasInboxEntries(inbox, sess))
}

func TestSessionHasInboxEntries_MatchingProject(t *testing.T) {
	dir := t.TempDir()
	inbox := session.NewInbox(dir)

	// Write an inbox entry for the project
	entry := &session.InboxEntry{
		Time:      time.Now(),
		Project:   "my-project",
		EntryType: session.EntryFYI,
		Message:   "Did some work",
	}
	require.NoError(t, inbox.Append(entry))

	sess := &session.Session{Project: "my-project"}
	assert.True(t, sessionHasInboxEntries(inbox, sess))
}

func TestSessionHasInboxEntries_DifferentProject(t *testing.T) {
	dir := t.TempDir()
	inbox := session.NewInbox(dir)

	entry := &session.InboxEntry{
		Time:      time.Now(),
		Project:   "other-project",
		EntryType: session.EntryFYI,
		Message:   "Did some work",
	}
	require.NoError(t, inbox.Append(entry))

	sess := &session.Session{Project: "my-project"}
	assert.False(t, sessionHasInboxEntries(inbox, sess))
}

func writeMinimalTranscript(t *testing.T, path string, userMsgCount int, hasEdits, hasCommands bool) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	for i := 0; i < userMsgCount; i++ {
		entry := map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role":    "user",
				"content": "test message",
			},
		}
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.WriteString("\n")
	}

	if hasEdits {
		entry := map[string]interface{}{
			"type": "assistant",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "tool_use", "name": "Edit", "input": map[string]string{"file_path": "/tmp/foo.go"}},
				},
			},
		}
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.WriteString("\n")
	}

	if hasCommands {
		for i := 0; i < 4; i++ {
			entry := map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"role": "assistant",
					"content": []map[string]interface{}{
						{"type": "tool_use", "name": "Bash", "input": map[string]string{"command": "echo hello"}},
					},
				},
			}
			data, _ := json.Marshal(entry)
			f.Write(data)
			f.WriteString("\n")
		}
	}
}
