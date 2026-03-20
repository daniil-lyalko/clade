# Auto-Dropbag Redesign — Implementation Plan

**Status**: Draft
**Branch**: `feat/auto-dropbag-v2`
**Date**: 2026-02-09
**Based on**: Research from claude-docs-researcher, cursor-docs-researcher, memory-expert

---

## Problem Statement

The current `auto-dropbag` Stop hook (disabled) fires after every Claude response and writes `DROPBAG-auto-{timestamp}.md` with git status + git log — identical to what `inject-context` already provides on SessionStart. Zero incremental value. Files accumulate forever.

## Design Decisions (from research)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Trigger** | Stop hook + PreCompact hook | Stop captures end-of-session; PreCompact captures mid-session "death boundary" before context compaction |
| **Data source** | Parse `transcript_path` JSONL | Contains every tool call, file edit, user message, assistant response |
| **Extraction** | Mechanical parsing + CLI LLM summarization | JSONL parsing in Go <1s even for 5MB; CLI summarization is $0 on Max/Pro plans |
| **CLI for summarization** | `claude -p` / `cursor-agent -p` based on `config.Agent` | Uses whichever agent the user configured; graceful fallback to mechanical-only |
| **Storage** | Single overwrite: `.clade/DROPBAG-auto.md` | Only latest matters; manual `/drop` continues using timestamped files |
| **Size target** | <500 tokens (LLM-compressed) or <1500 tokens (mechanical fallback) | Minimize injection tax; key info at top and bottom (lost-in-the-middle effect) |
| **Loop prevention** | Check `stop_hook_active` field from stdin JSON | Prevents recursion when CLI summarization triggers its own Stop hook |
| **Remote/missing binary** | `command -v clade` guard in hook command | Exit 0 silently when clade isn't installed |

## Architecture

```
Stop/PreCompact Hook Pipeline:

  stdin JSON ──► Parse stdin
                   │
                   ├─ stop_hook_active=true? → exit 0
                   │
                   ▼
               Parse JSONL transcript (streaming)    [3-5s]
                   │
                   ▼
               Build structured extract (~3K tokens)
                   │
                   ├─ CLI available? ──► Summarize   [10-30s]
                   │                     │
                   │                     ▼
                   │                  ~500 token narrative
                   │
                   ├─ No CLI? ──► Format extract as markdown
                   │               ~1500 tokens
                   │
                   ▼
               Write .clade/DROPBAG-auto.md (overwrite)
```

## Stdin JSON Schema (Stop Hook)

```json
{
  "session_id": "abc123",
  "transcript_path": "/Users/.../.claude/projects/.../<session-id>.jsonl",
  "cwd": "/Users/.../project",
  "permission_mode": "default",
  "hook_event_name": "Stop",
  "stop_hook_active": false
}
```

**Timeout**: 600s default for command hooks (not 30s — that's only prompt hooks).

## JSONL Transcript — What to Extract

| Entry Type | Content Block | Extract |
|-----------|---------------|---------|
| `assistant` | `tool_use` where `name=Edit` | `input.file_path` + edit count |
| `assistant` | `tool_use` where `name=Write` | `input.file_path` |
| `assistant` | `tool_use` where `name=Read` | `input.file_path` |
| `assistant` | `tool_use` where `name=Bash` | `input.command` |
| `assistant` | `tool_use` where `name=Task` | `input.description` (subagent spawns) |
| `assistant` | `text` | Last 2-3 text blocks (current state/next steps) |
| `user` | `text` (first message) | Session intent/goal |
| `user` | `tool_result` | Check for error patterns in content |
| `system` | `subtype=microcompact_boundary` | Compaction event count |
| `summary` | `summary` field | Compaction summaries (already LLM-generated!) |
| all | `timestamp` (first/last) | Session duration |
| all | `message.usage` | Token usage stats |

**Skip**: `thinking` blocks (verbose, signed), `isSidechain=true` entries (sub-agent chatter), `progress` entries (transient).

## Implementation: File Changes

### NEW: `internal/transcript/parser.go`

New package for JSONL transcript parsing. Standalone, testable, no dependencies on other clade internals.

```go
package transcript

type SessionExtract struct {
    SessionID    string
    Branch       string
    Duration     time.Duration
    StartTime    time.Time
    EndTime      time.Time

    // Tool activity
    FilesEdited  map[string]int   // path → edit count
    FilesWritten []string
    FilesRead    []string
    CommandsRun  []string         // last 20, deduped
    Errors       []string         // last 10, truncated to 200 chars

    // Narrative context
    UserIntent          string    // first user message, truncated to 500 chars
    LastAssistantMsgs   []string  // last 3 assistant text blocks, truncated to 300 chars each
    CompactionSummaries []string  // from type:"summary" entries
    UserPrompts         []string  // all user messages, first line only, max 10

    // Subagents
    SubagentTasks []SubagentInfo

    // Stats
    TotalToolUses   int
    CompactionCount int
    InputTokens     int64
    OutputTokens    int64
}

type SubagentInfo struct {
    Description string
    Duration    time.Duration
    ToolUses    int
    Tokens      int
}

// Parse reads a JSONL transcript file and extracts structured data.
// Streams line-by-line via bufio.Scanner to handle large files.
func Parse(path string) (*SessionExtract, error)
```

### NEW: `internal/transcript/format.go`

Two formatters:

```go
// FormatMarkdown produces structured markdown (~1500 tokens).
// Used as fallback when CLI summarization is unavailable.
func FormatMarkdown(extract *SessionExtract) string

// FormatPrompt produces prompt text for CLI summarization (~3000 tokens).
func FormatPrompt(extract *SessionExtract) string
```

**FormatMarkdown output** (mechanical fallback):
```markdown
# Session Context (auto-generated)

⚠️ From previous session — verify current state.

## Session Goal
> [first user message, truncated]

## What Happened
[compaction summaries if any, else last assistant messages]

## Files Modified
- path/to/file.go (edited 5x)
- path/to/new_file.go (created)

## Commands & Errors
- `go test ./...` → FAIL: TestFoo
- `go build ./...` → OK

## Last State
> [last assistant text block, truncated]
```

### NEW: `internal/transcript/parser_test.go`

Tests with fixture JSONL files in `internal/transcript/testdata/`.

### REWRITE: `internal/cmd/auto_dropbag.go`

Complete rewrite. New flow:

```go
func runAutoDROPBAG(cmd *cobra.Command, args []string) error {
    // 1. Read stdin JSON (Stop/PreCompact hook input)
    input, err := readStopHookInput()
    if err != nil {
        return nil // silent failure
    }

    // 2. Loop guard
    if input.StopHookActive {
        return nil
    }

    // 3. Validate transcript exists
    if input.TranscriptPath == "" {
        return nil
    }

    // 4. Debounce: compare transcript size vs last-processed
    if !shouldUpdate(input.CWD, input.TranscriptPath) {
        return nil
    }

    // 5. Parse transcript
    extract, err := transcript.Parse(input.TranscriptPath)
    if err != nil {
        return nil
    }

    // 6. Attempt CLI summarization
    summary, err := cliSummarize(extract)
    if err != nil {
        // Fallback: mechanical markdown
        summary = transcript.FormatMarkdown(extract)
    }

    // 7. Write .clade/DROPBAG-auto.md (overwrite)
    dropbagPath := filepath.Join(input.CWD, ".clade", "DROPBAG-auto.md")
    os.MkdirAll(filepath.Dir(dropbagPath), 0755)
    os.WriteFile(dropbagPath, []byte(summary), 0644)

    // 8. Update debounce state
    saveDebounceState(input.CWD, input.TranscriptPath)
    return nil
}
```

**CLI detection** uses `config.Agent` to choose `claude -p` or `cursor-agent -p`:
```go
func cliSummarize(extract *SessionExtract) (string, error) {
    cfg, _ := config.Load()

    // Detect CLI based on configured agent
    var cliCmd string
    switch {
    case cfg.Agent == "claude" || cfg.Agent == "":
        if _, err := exec.LookPath("claude"); err == nil {
            cliCmd = "claude"
        }
    case strings.Contains(cfg.Agent, "cursor"):
        if _, err := exec.LookPath("cursor-agent"); err == nil {
            cliCmd = "cursor-agent"
        }
    }

    // Fallback: try any available CLI
    if cliCmd == "" {
        for _, cmd := range []string{"claude", "cursor-agent"} {
            if _, err := exec.LookPath(cmd); err == nil {
                cliCmd = cmd
                break
            }
        }
    }

    if cliCmd == "" {
        return "", fmt.Errorf("no CLI available")
    }

    // Build prompt and run with timeout
    prompt := transcript.FormatPrompt(extract)
    ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx, cliCmd, "-p", "--output-format", "text",
        "Summarize this coding session for context resumption. Output ≤500 tokens of structured markdown with sections: Summary, Current State, Next Steps, Key Files. Be specific about file names and function names.")
    cmd.Stdin = strings.NewReader(prompt)
    output, err := cmd.Output()
    return string(output), err
}
```

### MODIFY: `internal/context/dropbag.go`

Update `ReadDropbag()` to read new auto file separately:

```go
type DropbagInfo struct {
    Manual    string        // from .clade/dropbags/DROPBAG-*.md
    Auto      string        // from .clade/DROPBAG-auto.md
    ManualAge time.Duration
    AutoAge   time.Duration
}
```

### MODIFY: `internal/context/format.go`

Render auto-dropbag separately from manual, with staleness label:

```markdown
## Session Context (from 20 minutes ago)
[auto-dropbag content]

## DROPBAG (from 2 hours ago)
[manual /drop content]
```

### MODIFY: `internal/cmd/setup.go`

Re-enable Stop hook with `command -v` guard. Add PreCompact hook for auto-dropbag:

```go
hooks := []hookDef{
    {"SessionStart hook", "clade inject-context", "pacer inject-context"},
    {"Stop hook (auto-dropbag)", "command -v clade >/dev/null 2>&1 && clade auto-dropbag || true", ""},
    {"PreCompact hook (auto-dropbag)", "command -v clade >/dev/null 2>&1 && clade auto-dropbag || true", ""},
    {"PreCompact hook (context-warning)", "clade context-warning", ""},
}
```

### MINOR: `internal/config/config.go`

Add optional config for auto-dropbag behavior:

```go
type AutoDropbagConfig struct {
    Enabled        bool `json:"enabled"`         // default: true
    UseLLM         bool `json:"useLLM"`          // default: true
    TimeoutSeconds int  `json:"timeoutSeconds"`  // default: 120
}
```

## Debounce Strategy (Revised)

**Old**: 60-second timer + git status check (wrong signal).

**New**: Transcript growth check + 5-minute minimum interval.

State file: `.clade/.dropbag-state.json`
```json
{
  "transcriptSize": 245000,
  "lastWritten": "2026-02-09T16:30:00Z"
}
```

## Graceful Degradation Chain

```
Stop hook fires
  ├─ clade not installed? → exit 0 (command -v guard)
  ├─ stop_hook_active=true? → exit 0
  ├─ transcript_path empty? → exit 0
  ├─ transcript hasn't grown? → exit 0 (debounce)
  ├─ JSONL parse fails? → exit 0 (silent)
  ├─ CLI available? → full LLM summary (~500 tokens)
  └─ No CLI? → mechanical markdown (~1500 tokens)
```

Every failure mode exits silently. The hook never breaks the user's session.

## New File Layout

```
internal/
├── transcript/              ← NEW package
│   ├── parser.go            ← JSONL streaming parser
│   ├── parser_test.go       ← Tests with fixture data
│   ├── format.go            ← Markdown + prompt formatters
│   └── testdata/            ← Fixture JSONL files
│       ├── small_session.jsonl
│       ├── large_session.jsonl
│       └── edge_cases.jsonl
├── cmd/
│   ├── auto_dropbag.go      ← REWRITE
│   ├── auto_dropbag_test.go ← NEW
│   ├── setup.go             ← MODIFY (re-enable Stop, add PreCompact)
│   └── inject.go            ← MINOR
├── context/
│   ├── dropbag.go           ← MODIFY (read .clade/DROPBAG-auto.md)
│   └── format.go            ← MODIFY (render auto vs manual separately)
└── config/
    └── config.go            ← MINOR (add AutoDropbagConfig)
```

## Implementation Phases

### Phase 1: Mechanical Foundation (MVP)
1. `internal/transcript/parser.go` — JSONL parser + tests
2. `internal/transcript/format.go` — Markdown formatter (mechanical)
3. Rewrite `internal/cmd/auto_dropbag.go` — read stdin, parse transcript, write file
4. Update `internal/context/dropbag.go` — read new DROPBAG-auto.md
5. Update `internal/context/format.go` — render with staleness label
6. Update `internal/cmd/setup.go` — re-enable Stop hook with command -v guard
7. Test end-to-end manually

**Deliverable**: Working auto-dropbag that produces ~1500 token mechanical summaries. No LLM dependency.

### Phase 2: CLI Summarization
8. Add `cliSummarize()` to auto_dropbag.go — detect CLI, run headless
9. Add `FormatPrompt()` to transcript/format.go — prompt for CLI
10. Add `AutoDropbagConfig` to config.go — user controls
11. Test with real Claude CLI

**Deliverable**: Auto-dropbag produces ~500 token LLM-compressed summaries when CLI is available.

### Phase 3: PreCompact + Cursor
12. Register PreCompact hook in setup.go (both Claude and Cursor)
13. Register Stop hook for Cursor in setup.go
14. Handle `hook_event_name` differences between Stop and PreCompact
15. Test Cursor hooks end-to-end

**Deliverable**: Full dual-agent support with mid-session capture.

### Phase 4: Polish
16. `clade doctor` — check if auto-dropbag hook is registered
17. Migration path — clean up old DROPBAG-auto-*.md timestamped files
18. Documentation updates (USER_GUIDE.md, ARCHITECTURE.md)

## Open Questions

1. **Same file for PreCompact and Stop?** Both write `.clade/DROPBAG-auto.md`. Latest wins. Simpler.

2. **Thinking blocks?** Skip for v1 — verbose, includes signatures.

3. **`claude -p` stdin vs args?** Pipe extract via stdin to avoid arg length limits: `echo "$extract" | claude -p "Summarize..."`

4. **Gitignore?** Add `.clade/DROPBAG-auto.md` and `.clade/.dropbag-state.json` — machine-generated, ephemeral.

5. **Existing `--agent` flag?** auto-dropbag reads `config.Agent` directly. The flag is for interactive commands.
