package transcript

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FormatMarkdown produces a mechanical markdown summary (~1500 tokens) of the session.
// Used as fallback when CLI summarization is unavailable.
func FormatMarkdown(extract *SessionExtract) string {
	var buf strings.Builder

	buf.WriteString("# Session Context (auto-generated)\n\n")
	buf.WriteString("_From previous session — verify current state before acting._\n\n")

	// Session goal (user intent)
	if extract.UserIntent != "" {
		buf.WriteString("## Session Goal\n")
		buf.WriteString(fmt.Sprintf("> %s\n\n", singleLine(extract.UserIntent)))
	}

	// What happened — prefer compaction summaries, fallback to last assistant msgs
	if len(extract.CompactionSummaries) > 0 || len(extract.LastAssistantMsgs) > 0 {
		buf.WriteString("## What Happened\n\n")
		if len(extract.CompactionSummaries) > 0 {
			for _, s := range extract.CompactionSummaries {
				buf.WriteString(fmt.Sprintf("- %s\n", singleLine(s)))
			}
		} else {
			for _, msg := range extract.LastAssistantMsgs {
				buf.WriteString(fmt.Sprintf("> %s\n\n", singleLine(msg)))
			}
		}
		buf.WriteString("\n")
	}

	// Files modified
	if len(extract.FilesEdited) > 0 || len(extract.FilesWritten) > 0 {
		buf.WriteString("## Files Modified\n\n")

		// Sort edited files by edit count (most edited first)
		type editEntry struct {
			Path  string
			Count int
		}
		var edits []editEntry
		for p, c := range extract.FilesEdited {
			edits = append(edits, editEntry{p, c})
		}
		sort.Slice(edits, func(i, j int) bool {
			return edits[i].Count > edits[j].Count
		})

		for _, e := range edits {
			buf.WriteString(fmt.Sprintf("- %s (edited %dx)\n", e.Path, e.Count))
		}
		for _, f := range extract.FilesWritten {
			// Only show if not already in edits
			if _, inEdits := extract.FilesEdited[f]; !inEdits {
				buf.WriteString(fmt.Sprintf("- %s (created)\n", f))
			}
		}
		buf.WriteString("\n")
	}

	// Files read (compact)
	if len(extract.FilesRead) > 0 {
		buf.WriteString("## Files Read\n\n")
		// Limit to 15 files to keep output manageable
		shown := extract.FilesRead
		if len(shown) > 15 {
			shown = shown[:15]
		}
		buf.WriteString("- " + strings.Join(shown, ", "))
		if len(extract.FilesRead) > 15 {
			buf.WriteString(fmt.Sprintf(" (+%d more)", len(extract.FilesRead)-15))
		}
		buf.WriteString("\n\n")
	}

	// Commands & errors
	if len(extract.CommandsRun) > 0 || len(extract.Errors) > 0 {
		buf.WriteString("## Commands & Errors\n\n")
		// Show last 10 commands
		cmds := extract.CommandsRun
		if len(cmds) > 10 {
			cmds = cmds[len(cmds)-10:]
		}
		for _, cmd := range cmds {
			buf.WriteString(fmt.Sprintf("- `%s`\n", singleLine(cmd)))
		}
		if len(extract.Errors) > 0 {
			buf.WriteString("\n**Errors:**\n")
			for _, e := range extract.Errors {
				buf.WriteString(fmt.Sprintf("- %s\n", singleLine(e)))
			}
		}
		buf.WriteString("\n")
	}

	// Last state
	if len(extract.LastAssistantMsgs) > 0 {
		buf.WriteString("## Last State\n\n")
		last := extract.LastAssistantMsgs[len(extract.LastAssistantMsgs)-1]
		buf.WriteString(fmt.Sprintf("> %s\n\n", singleLine(last)))
	}

	// Session stats (compact)
	var stats []string
	if extract.TotalToolUses > 0 {
		stats = append(stats, fmt.Sprintf("%d tool uses", extract.TotalToolUses))
	}
	if extract.CompactionCount > 0 {
		stats = append(stats, fmt.Sprintf("%d compactions", extract.CompactionCount))
	}
	if len(extract.SubagentTasks) > 0 {
		stats = append(stats, fmt.Sprintf("%d subagent tasks", len(extract.SubagentTasks)))
	}
	if !extract.StartTime.IsZero() && !extract.EndTime.IsZero() {
		duration := extract.EndTime.Sub(extract.StartTime)
		if duration > 0 {
			stats = append(stats, fmt.Sprintf("duration: %s", formatDuration(duration)))
		}
	}
	if len(stats) > 0 {
		buf.WriteString(fmt.Sprintf("_Session stats: %s_\n", strings.Join(stats, ", ")))
	}

	return buf.String()
}

// FormatPrompt produces a structured prompt for CLI-based LLM summarization.
func FormatPrompt(extract *SessionExtract) string {
	var buf strings.Builder

	buf.WriteString(`Summarize this coding session for context resumption in a new session.
Output ≤500 tokens of markdown with these sections:
- Summary (what was accomplished)
- Current State (what works, what's broken)
- Next Steps (specific file/function names)
- Key Files (files to look at first)

Do NOT include a top-level heading. Start directly with ## Summary.

SESSION DATA:
`)

	if extract.UserIntent != "" {
		buf.WriteString(fmt.Sprintf("\nUser Intent: %s\n", extract.UserIntent))
	}

	if extract.Branch != "" {
		buf.WriteString(fmt.Sprintf("Branch: %s\n", extract.Branch))
	}

	// User prompts (shows the conversation flow)
	if len(extract.UserPrompts) > 0 {
		buf.WriteString("\nUser Messages:\n")
		for _, p := range extract.UserPrompts {
			buf.WriteString(fmt.Sprintf("- %s\n", p))
		}
	}

	// Compaction summaries
	if len(extract.CompactionSummaries) > 0 {
		buf.WriteString("\nSession Summaries (from context compactions):\n")
		for _, s := range extract.CompactionSummaries {
			buf.WriteString(fmt.Sprintf("- %s\n", singleLine(s)))
		}
	}

	// Files
	if len(extract.FilesEdited) > 0 {
		buf.WriteString("\nFiles Edited:\n")
		for p, c := range extract.FilesEdited {
			buf.WriteString(fmt.Sprintf("- %s (%dx)\n", p, c))
		}
	}
	if len(extract.FilesWritten) > 0 {
		buf.WriteString("\nFiles Created:\n")
		for _, f := range extract.FilesWritten {
			buf.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	// Commands and errors
	if len(extract.CommandsRun) > 0 {
		buf.WriteString("\nCommands Run (last 10):\n")
		cmds := extract.CommandsRun
		if len(cmds) > 10 {
			cmds = cmds[len(cmds)-10:]
		}
		for _, c := range cmds {
			buf.WriteString(fmt.Sprintf("- %s\n", singleLine(c)))
		}
	}
	if len(extract.Errors) > 0 {
		buf.WriteString("\nErrors Encountered:\n")
		for _, e := range extract.Errors {
			buf.WriteString(fmt.Sprintf("- %s\n", singleLine(e)))
		}
	}

	// Last assistant messages
	if len(extract.LastAssistantMsgs) > 0 {
		buf.WriteString("\nLast Assistant Messages:\n")
		for _, msg := range extract.LastAssistantMsgs {
			buf.WriteString(fmt.Sprintf("---\n%s\n", msg))
		}
	}

	return buf.String()
}

// --- helpers ---

// singleLine collapses multiline text to a single line for compact output.
func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// formatDuration returns a human-readable duration string.
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
