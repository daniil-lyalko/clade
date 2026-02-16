package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionExtract holds meaningful data extracted from a Claude Code JSONL transcript.
type SessionExtract struct {
	SessionID string
	Branch    string
	StartTime time.Time
	EndTime   time.Time

	FilesEdited  map[string]int // path → edit count
	FilesWritten []string
	FilesRead    []string
	CommandsRun  []string // last 20
	Errors       []string // last 10, truncated 200 chars each

	UserIntent          string   // first real user text message, truncated 500 chars
	LastAssistantMsgs   []string // last 3 text blocks, truncated 300 chars each
	CompactionSummaries []string // from type:"summary" entries
	UserPrompts         []string // first line of each user message, max 10

	SubagentTasks []SubagentInfo
	TotalToolUses int
	CompactionCount int
	InputTokens   int64
	OutputTokens  int64
}

// SubagentInfo tracks a Task tool invocation.
type SubagentInfo struct {
	Description string
	AgentType   string
}

// Parse reads a Claude Code JSONL transcript and extracts session context.
// Designed to handle large files efficiently by streaming line-by-line.
func Parse(path string) (*SessionExtract, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	extract := &SessionExtract{
		FilesEdited: make(map[string]int),
	}

	scanner := bufio.NewScanner(f)
	// JSONL lines with tool results can be very large (file contents, etc.)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	filesReadSet := make(map[string]bool)
	commandsAll := make([]string, 0, 64)
	errorsAll := make([]string, 0, 16)
	assistantMsgs := make([]string, 0, 16)
	userPromptCount := 0
	foundIntent := false

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip corrupt lines
		}
		entry.Message.parseContent()

		// Skip sidechain entries (subagent internal messages)
		if entry.IsSidechain {
			continue
		}

		// Skip progress and file-history-snapshot entries
		if entry.Type == "progress" || entry.Type == "file-history-snapshot" {
			continue
		}

		// Track session metadata from first entry
		if extract.SessionID == "" && entry.SessionID != "" {
			extract.SessionID = entry.SessionID
		}
		if extract.Branch == "" && entry.GitBranch != "" {
			extract.Branch = entry.GitBranch
		}

		// Track timestamps
		if !entry.Timestamp.IsZero() {
			ts := entry.Timestamp.Time
			if extract.StartTime.IsZero() || ts.Before(extract.StartTime) {
				extract.StartTime = ts
			}
			if ts.After(extract.EndTime) {
				extract.EndTime = ts
			}
		}

		switch entry.Type {
		case "assistant":
			processAssistant(&entry, extract, filesReadSet, &commandsAll, &assistantMsgs)

		case "user":
			processUser(&entry, extract, &errorsAll, &foundIntent, &userPromptCount)

		case "summary":
			if entry.Summary != "" {
				extract.CompactionSummaries = append(extract.CompactionSummaries, entry.Summary)
			}

		case "system":
			if entry.Subtype == "microcompact_boundary" {
				extract.CompactionCount++
			}
		}
	}

	// Finalize: trim to limits
	if len(commandsAll) > 20 {
		extract.CommandsRun = commandsAll[len(commandsAll)-20:]
	} else {
		extract.CommandsRun = commandsAll
	}

	if len(errorsAll) > 10 {
		extract.Errors = errorsAll[len(errorsAll)-10:]
	} else {
		extract.Errors = errorsAll
	}

	if len(assistantMsgs) > 3 {
		extract.LastAssistantMsgs = assistantMsgs[len(assistantMsgs)-3:]
	} else {
		extract.LastAssistantMsgs = assistantMsgs
	}

	// Deduplicate and sort files read
	for f := range filesReadSet {
		extract.FilesRead = append(extract.FilesRead, f)
	}
	sort.Strings(extract.FilesRead)

	// Sort files written
	sort.Strings(extract.FilesWritten)

	return extract, scanner.Err()
}

// processAssistant extracts tool uses and text from assistant messages.
func processAssistant(entry *jsonEntry, extract *SessionExtract, filesReadSet map[string]bool, commandsAll *[]string, assistantMsgs *[]string) {
	// Track token usage
	if entry.Message.Usage.InputTokens > 0 {
		extract.InputTokens += entry.Message.Usage.InputTokens
	}
	if entry.Message.Usage.OutputTokens > 0 {
		extract.OutputTokens += entry.Message.Usage.OutputTokens
	}

	for _, block := range entry.Message.Content {
		switch block.Type {
		case "tool_use":
			extract.TotalToolUses++
			processToolUse(&block, extract, filesReadSet, commandsAll)

		case "text":
			if text := strings.TrimSpace(block.Text); text != "" {
				*assistantMsgs = append(*assistantMsgs, truncate(text, 300))
			}
		}
	}
}

// processToolUse extracts file/command info from a tool_use block.
func processToolUse(block *contentBlock, extract *SessionExtract, filesReadSet map[string]bool, commandsAll *[]string) {
	switch block.Name {
	case "Edit":
		if fp := block.Input.FilePath; fp != "" {
			extract.FilesEdited[shortenPath(fp)]++
		}

	case "Write":
		if fp := block.Input.FilePath; fp != "" {
			short := shortenPath(fp)
			extract.FilesWritten = appendUnique(extract.FilesWritten, short)
		}

	case "Read":
		if fp := block.Input.FilePath; fp != "" {
			filesReadSet[shortenPath(fp)] = true
		}

	case "Bash":
		if cmd := block.Input.Command; cmd != "" {
			*commandsAll = append(*commandsAll, truncate(cmd, 200))
		}

	case "Glob", "Grep":
		// Count as tool use but don't need detailed extraction

	case "Task":
		info := SubagentInfo{
			Description: block.Input.Description,
			AgentType:   block.Input.SubagentType,
		}
		if info.Description != "" || info.AgentType != "" {
			extract.SubagentTasks = append(extract.SubagentTasks, info)
		}
	}
}

// processUser extracts intent, prompts, and errors from user messages.
func processUser(entry *jsonEntry, extract *SessionExtract, errorsAll *[]string, foundIntent *bool, userPromptCount *int) {
	for _, block := range entry.Message.Content {
		switch block.Type {
		case "text":
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			// Skip system-injected messages (hooks, commands, reminders)
			if isSystemInjected(text) {
				continue
			}

			if !*foundIntent {
				extract.UserIntent = truncate(text, 500)
				*foundIntent = true
			}

			if *userPromptCount < 10 {
				firstLine := text
				if idx := strings.IndexByte(text, '\n'); idx > 0 {
					firstLine = text[:idx]
				}
				extract.UserPrompts = append(extract.UserPrompts, truncate(firstLine, 120))
				*userPromptCount++
			}

		case "tool_result":
			if block.IsError {
				content := block.ToolResultContent()
				if content != "" {
					*errorsAll = append(*errorsAll, truncate(content, 200))
				}
			}
		}
	}
}

// isSystemInjected returns true if a user message is system-injected (not genuine user input).
func isSystemInjected(text string) bool {
	prefixes := []string{
		"<local-command-",
		"<command-name>",
		"<system-reminder>",
		"<local-command-stdout>",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

// --- JSON structures for minimal unmarshaling ---

type jsonEntry struct {
	Type        string       `json:"type"`
	SessionID   string       `json:"sessionId"`
	GitBranch   string       `json:"gitBranch"`
	IsSidechain bool         `json:"isSidechain"`
	Timestamp   jsonTime     `json:"timestamp"`
	Subtype     string       `json:"subtype"`
	Summary     string       `json:"summary"`
	Message     entryMessage `json:"message"`
}

type entryMessage struct {
	Role       string         `json:"role"`
	RawContent json.RawMessage `json:"content"`
	Content    []contentBlock `json:"-"` // populated after unmarshaling
	Usage      usageInfo      `json:"usage"`
}

func (m *entryMessage) parseContent() {
	if m.RawContent == nil {
		return
	}

	// Try as array of content blocks
	var blocks []contentBlock
	if err := json.Unmarshal(m.RawContent, &blocks); err == nil {
		m.Content = blocks
		return
	}

	// Try as plain string (user text messages)
	var s string
	if err := json.Unmarshal(m.RawContent, &s); err == nil && s != "" {
		m.Content = []contentBlock{{Type: "text", Text: s}}
		return
	}
}

type contentBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Name    string          `json:"name,omitempty"`    // tool_use name
	Input   toolInput       `json:"input,omitempty"`   // tool_use input
	IsError bool            `json:"is_error,omitempty"`
	Content json.RawMessage `json:"content,omitempty"` // tool_result content (string or array)
}

// ToolResultContent extracts the text content from a tool_result block.
func (b *contentBlock) ToolResultContent() string {
	if b.Content == nil {
		return ""
	}

	// Try as string first
	var s string
	if err := json.Unmarshal(b.Content, &s); err == nil {
		return s
	}

	// Try as array of {type, text} blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b.Content, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	return ""
}

type toolInput struct {
	FilePath    string `json:"file_path,omitempty"`
	Command     string `json:"command,omitempty"`
	Description string `json:"description,omitempty"`
	SubagentType string `json:"subagent_type,omitempty"`
}

type usageInfo struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// jsonTime handles ISO 8601 timestamp parsing.
type jsonTime struct {
	time.Time
}

func (jt *jsonTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Not a string, skip
		jt.Time = time.Time{}
		return nil
	}
	if s == "" {
		jt.Time = time.Time{}
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// Try RFC3339 without nanos
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			jt.Time = time.Time{}
			return nil // don't fail on bad timestamps
		}
	}
	jt.Time = t
	return nil
}

func (jt jsonTime) IsZero() bool {
	return jt.Time.IsZero()
}

// --- helpers ---

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func shortenPath(p string) string {
	// Replace home directory with ~
	if home, err := os.UserHomeDir(); err == nil {
		if strings.HasPrefix(p, home) {
			return "~" + p[len(home):]
		}
	}
	// For absolute paths that don't match home, use path relative to common prefixes
	return filepath.Clean(p)
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}
