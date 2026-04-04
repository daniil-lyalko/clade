package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
