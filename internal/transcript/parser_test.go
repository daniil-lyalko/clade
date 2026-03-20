package transcript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_FullSession(t *testing.T) {
	fixturePath := filepath.Join("testdata", "session.jsonl")
	extract, err := Parse(fixturePath)

	require.NoError(t, err)
	require.NotNil(t, extract)

	// Session metadata
	assert.Equal(t, "test-session-123", extract.SessionID)
	assert.Equal(t, "feat/auto-dropbag", extract.Branch)
	assert.False(t, extract.StartTime.IsZero())
	assert.False(t, extract.EndTime.IsZero())
	assert.True(t, extract.EndTime.After(extract.StartTime))

	// Files edited — auto_dropbag.go was edited twice (two Edit tool_use blocks)
	// Path won't be shortened since /Users/test isn't the real home dir
	assert.Contains(t, extract.FilesEdited, "/Users/test/project/internal/cmd/auto_dropbag.go")
	assert.Equal(t, 2, extract.FilesEdited["/Users/test/project/internal/cmd/auto_dropbag.go"])

	// Files written
	assert.Contains(t, extract.FilesWritten, "/Users/test/project/internal/transcript/parser.go")

	// Files read
	assert.Contains(t, extract.FilesRead, "/Users/test/project/internal/cmd/auto_dropbag.go")

	// Commands run
	assert.GreaterOrEqual(t, len(extract.CommandsRun), 3) // go build, go test x2
	assert.Contains(t, extract.CommandsRun, "go build ./...")

	// Errors — the test failure
	assert.Len(t, extract.Errors, 1)
	assert.Contains(t, extract.Errors[0], "FAIL: TestParse")

	// User intent — first real user message
	assert.Contains(t, extract.UserIntent, "auto-dropbag redesign")

	// User prompts
	assert.GreaterOrEqual(t, len(extract.UserPrompts), 2)

	// Last assistant messages (last 3 text blocks)
	assert.LessOrEqual(t, len(extract.LastAssistantMsgs), 3)
	assert.Contains(t, extract.LastAssistantMsgs[len(extract.LastAssistantMsgs)-1], "tests pass now")

	// Compaction summaries
	assert.Len(t, extract.CompactionSummaries, 1)
	assert.Contains(t, extract.CompactionSummaries[0], "transcript parser")

	// Subagent tasks
	assert.Len(t, extract.SubagentTasks, 1)
	assert.Equal(t, "Explore parser", extract.SubagentTasks[0].Description)
	assert.Equal(t, "Explore", extract.SubagentTasks[0].AgentType)

	// Token usage
	assert.Greater(t, extract.TotalToolUses, 0)
	assert.Greater(t, extract.InputTokens, int64(0))
	assert.Greater(t, extract.OutputTokens, int64(0))
}

func TestParse_SkipsSidechain(t *testing.T) {
	fixturePath := filepath.Join("testdata", "session.jsonl")
	extract, err := Parse(fixturePath)

	require.NoError(t, err)

	// The sidechain assistant message should not appear in LastAssistantMsgs
	for _, msg := range extract.LastAssistantMsgs {
		assert.NotContains(t, msg, "Sidechain subagent")
	}
}

func TestParse_SkipsSystemInjectedMessages(t *testing.T) {
	fixturePath := filepath.Join("testdata", "session.jsonl")
	extract, err := Parse(fixturePath)

	require.NoError(t, err)

	// System-injected messages (like <command-name>/clear) should not be user intent
	assert.NotContains(t, extract.UserIntent, "<command-name>")
	assert.NotContains(t, extract.UserIntent, "<local-command-")
}

func TestParse_EmptyFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "empty.jsonl")
	require.NoError(t, os.WriteFile(tmpFile, []byte(""), 0644))

	extract, err := Parse(tmpFile)

	require.NoError(t, err)
	assert.Empty(t, extract.SessionID)
	assert.Empty(t, extract.FilesEdited)
	assert.Empty(t, extract.CommandsRun)
}

func TestParse_CorruptLine(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "corrupt.jsonl")
	content := `{"type":"user","sessionId":"s1","message":{"role":"user","content":"Hello"},"timestamp":"2026-02-10T10:00:00Z"}
not valid json at all
{"type":"assistant","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"Hi there"}],"usage":{"input_tokens":10,"output_tokens":5}},"timestamp":"2026-02-10T10:00:05Z"}
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	extract, err := Parse(tmpFile)

	require.NoError(t, err)
	assert.Equal(t, "s1", extract.SessionID)
	// Should still get the assistant message despite corrupt line in middle
	assert.Len(t, extract.LastAssistantMsgs, 1)
	assert.Contains(t, extract.LastAssistantMsgs[0], "Hi there")
}

func TestParse_SingleLine(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "single.jsonl")
	content := `{"type":"user","sessionId":"solo","gitBranch":"main","message":{"role":"user","content":"Fix the login bug"},"timestamp":"2026-02-10T10:00:00Z"}
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	extract, err := Parse(tmpFile)

	require.NoError(t, err)
	assert.Equal(t, "solo", extract.SessionID)
	assert.Equal(t, "main", extract.Branch)
	assert.Contains(t, extract.UserIntent, "Fix the login bug")
}

func TestParse_NonexistentFile(t *testing.T) {
	_, err := Parse("/nonexistent/path/transcript.jsonl")
	assert.Error(t, err)
}

func TestParse_CommandsLimitedTo20(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "many_commands.jsonl")

	var lines string
	for i := 0; i < 25; i++ {
		lines += `{"type":"assistant","sessionId":"s1","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"echo cmd` + string(rune('A'+i)) + `"}}],"usage":{"input_tokens":10,"output_tokens":5}},"timestamp":"2026-02-10T10:00:00Z"}` + "\n"
	}
	require.NoError(t, os.WriteFile(tmpFile, []byte(lines), 0644))

	extract, err := Parse(tmpFile)
	require.NoError(t, err)
	assert.Len(t, extract.CommandsRun, 20) // Capped at 20
}

func TestFormatMarkdown_Basic(t *testing.T) {
	extract := &SessionExtract{
		UserIntent:  "Implement feature X",
		Branch:      "feat/x",
		FilesEdited: map[string]int{"main.go": 3, "util.go": 1},
		FilesWritten: []string{"new.go"},
		FilesRead:   []string{"config.go", "readme.md"},
		CommandsRun: []string{"go build ./...", "go test ./..."},
		Errors:      []string{"test failed: expected 5 got 3"},
		LastAssistantMsgs: []string{"All tests pass now."},
		TotalToolUses: 15,
	}

	output := FormatMarkdown(extract)

	assert.Contains(t, output, "Session Context (auto-generated)")
	assert.Contains(t, output, "Implement feature X")
	assert.Contains(t, output, "main.go (edited 3x)")
	assert.Contains(t, output, "new.go (created)")
	assert.Contains(t, output, "go build ./...")
	assert.Contains(t, output, "test failed")
	assert.Contains(t, output, "All tests pass now")
	assert.Contains(t, output, "15 tool uses")
}

func TestFormatPrompt_Basic(t *testing.T) {
	extract := &SessionExtract{
		UserIntent:  "Fix login bug",
		Branch:      "fix/login",
		FilesEdited: map[string]int{"auth.go": 2},
		CommandsRun: []string{"go test ./auth/..."},
		Errors:      []string{"auth test failed"},
	}

	output := FormatPrompt(extract)

	assert.Contains(t, output, "Summarize this coding session")
	assert.Contains(t, output, "Fix login bug")
	assert.Contains(t, output, "auth.go")
	assert.Contains(t, output, "auth test failed")
}

func TestToolResultContent_String(t *testing.T) {
	block := &contentBlock{
		Type:    "tool_result",
		Content: []byte(`"file contents here"`),
	}
	assert.Equal(t, "file contents here", block.ToolResultContent())
}

func TestToolResultContent_Array(t *testing.T) {
	block := &contentBlock{
		Type:    "tool_result",
		Content: []byte(`[{"type":"text","text":"first"},{"type":"text","text":"second"}]`),
	}
	assert.Equal(t, "first\nsecond", block.ToolResultContent())
}

func TestToolResultContent_Nil(t *testing.T) {
	block := &contentBlock{
		Type: "tool_result",
	}
	assert.Equal(t, "", block.ToolResultContent())
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "short", truncate("short", 100))
	assert.Equal(t, "12345...", truncate("1234567890", 5))
	assert.Equal(t, "", truncate("", 10))
}

func TestSingleLine(t *testing.T) {
	assert.Equal(t, "hello world", singleLine("hello\nworld"))
	assert.Equal(t, "a b c", singleLine("a  b  c"))
	assert.Equal(t, "trimmed", singleLine("  trimmed  "))
}
