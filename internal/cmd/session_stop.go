package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/session"
	"github.com/spf13/cobra"
)

type triageTier int

const (
	tierTrivial triageTier = iota
	tierLight
	tierAsync
)

var sessionStopCmd = &cobra.Command{
	Use:    "session-stop",
	Short:  "Update session registry on stop (called by Stop hook)",
	Hidden: true,
	RunE:   runSessionStop,
}

func init() {
	rootCmd.AddCommand(sessionStopCmd)
}

func runSessionStop(cmd *cobra.Command, args []string) error {
	input, err := readStopHookInput()
	if err != nil {
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	// Loop guard
	if input.StopHookActive {
		return nil
	}

	baseDir := cladeBaseDir()
	reg := session.NewRegistry(baseDir)
	inbox := session.NewInbox(baseDir)

	return doSessionStop(reg, inbox, input)
}

// doSessionStop performs the three-tier triage and updates the session.
func doSessionStop(reg *session.Registry, inbox *session.Inbox, input *stopHookInput) error {
	sess, err := reg.Get(input.SessionID)
	if err != nil {
		// Session not registered (possibly started before v0.8). Create a minimal record.
		sess = &session.Session{
			SessionID: input.SessionID,
			CWD:       input.CWD,
			Project:   detectProjectName(input.CWD),
			Started:   time.Now(),
		}
	}

	// Read last 5KB of transcript for triage
	userMsgs, hasEdits, hasCommands := quickTranscriptScan(input.TranscriptPath)

	// Check if inbox entries were already written during this session
	hasInbox := sessionHasInboxEntries(inbox, sess)

	tier := triageSession(userMsgs, hasEdits, hasCommands, hasInbox)

	switch tier {
	case tierTrivial:
		// Just mark stopped. <10ms.
		sess.Status = session.StatusStopped
		sess.LastActive = time.Now()
		return reg.Save(sess)

	case tierLight:
		// Update status + grab summary from last assistant message. <50ms.
		sess.Status = session.StatusStopped
		sess.LastActive = time.Now()
		sess.Summary = extractQuickSummary(input.TranscriptPath)
		return reg.Save(sess)

	case tierAsync:
		// Mark as stopping, fork background process, return immediately. <50ms.
		sess.Status = session.StatusStopping
		sess.LastActive = time.Now()
		if err := reg.Save(sess); err != nil {
			return err
		}
		return forkAsync(input)
	}

	return nil
}

// triageSession determines the processing tier for the stop hook.
func triageSession(userMsgs int, hasEdits, hasCommands, hasInbox bool) triageTier {
	trivial := userMsgs < 3 && !hasEdits && !hasCommands
	if trivial {
		return tierTrivial
	}

	needsAsync := (hasEdits || hasCommands) && !hasInbox
	if needsAsync {
		return tierAsync
	}

	return tierLight
}

// quickTranscriptScan reads the last 5KB of the transcript JSONL and extracts
// triage heuristics: user message count, presence of edits, presence of commands (>3 Bash).
func quickTranscriptScan(path string) (userMsgs int, hasEdits bool, hasCommands bool) {
	if path == "" {
		return 0, false, false
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, false, false
	}
	defer f.Close()

	// Seek to last 5KB
	const tailSize = 5 * 1024
	info, err := f.Stat()
	if err != nil {
		return 0, false, false
	}
	if info.Size() > tailSize {
		f.Seek(info.Size()-tailSize, io.SeekStart)
		// Skip partial first line
		reader := bufio.NewReader(f)
		reader.ReadString('\n')
		scanTail(reader, &userMsgs, &hasEdits, &hasCommands)
	} else {
		reader := bufio.NewReader(f)
		scanTail(reader, &userMsgs, &hasEdits, &hasCommands)
	}

	return
}

func scanTail(reader *bufio.Reader, userMsgs *int, hasEdits *bool, hasCommands *bool) {
	bashCount := 0
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Lightweight JSON check without full unmarshal
		lineStr := string(line)
		if strings.Contains(lineStr, `"type":"user"`) || strings.Contains(lineStr, `"type": "user"`) {
			*userMsgs++
		}
		if strings.Contains(lineStr, `"name":"Edit"`) || strings.Contains(lineStr, `"name": "Edit"`) ||
			strings.Contains(lineStr, `"name":"Write"`) || strings.Contains(lineStr, `"name": "Write"`) {
			*hasEdits = true
		}
		if strings.Contains(lineStr, `"name":"Bash"`) || strings.Contains(lineStr, `"name": "Bash"`) {
			bashCount++
		}
	}

	*hasCommands = bashCount > 3
}

// sessionHasInboxEntries checks if today's inbox file contains entries from this session's project.
func sessionHasInboxEntries(inbox *session.Inbox, sess *session.Session) bool {
	entries, _, err := inbox.ReadRecent(0)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Project == sess.Project {
			return true
		}
	}
	return false
}

// extractQuickSummary reads the last few KB of transcript and returns the
// last assistant text message as a summary.
func extractQuickSummary(path string) string {
	if path == "" {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	const tailSize = 10 * 1024
	info, err := f.Stat()
	if err != nil {
		return ""
	}

	var reader *bufio.Reader
	if info.Size() > tailSize {
		f.Seek(info.Size()-tailSize, io.SeekStart)
		reader = bufio.NewReader(f)
		reader.ReadString('\n') // skip partial line
	} else {
		reader = bufio.NewReader(f)
	}

	var lastAssistantText string
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "assistant" {
			continue
		}

		// Try content as array of blocks
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(entry.Message.Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
					lastAssistantText = b.Text
				}
			}
			continue
		}

		// Try content as string
		var s string
		if err := json.Unmarshal(entry.Message.Content, &s); err == nil && s != "" {
			lastAssistantText = s
		}
	}

	// Truncate to 300 chars
	if len(lastAssistantText) > 300 {
		lastAssistantText = lastAssistantText[:300] + "..."
	}

	return lastAssistantText
}

// forkAsync launches `clade session-stop-async` as a detached background process.
func forkAsync(input *stopHookInput) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find clade executable: %w", err)
	}

	// Pass session info as JSON via env var (stdin won't be available to background process)
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "session-stop-async")
	cmd.Env = append(os.Environ(), "CLADE_STOP_INPUT="+string(inputJSON))
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Detach from parent process group
	cmd.SysProcAttr = detachedProcAttr()

	return cmd.Start()
}
