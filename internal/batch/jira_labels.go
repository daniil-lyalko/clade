package batch

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// LabelQueryRunner executes a Claude one-shot query and returns stdout.
// Abstracted so tests can inject canned responses without shelling out.
type LabelQueryRunner func(prompt string) ([]byte, error)

// labelQueryResponse is the JSON contract clade asks Claude to emit.
//
// Claude must respond with a single line of the form:
//
//	{"tickets":["PROJ-1","PROJ-2"],"error":null}
//
// On failure (MCP unavailable, query error) Claude emits:
//
//	{"tickets":[],"error":"<brief reason>"}
type labelQueryResponse struct {
	Tickets []string `json:"tickets"`
	Error   *string  `json:"error"`
}

// FetchTicketsByLabel resolves Jira ticket IDs matching the given labels
// (and optional project keys) by delegating the query to Claude via the
// Jira MCP. Returns TicketInput values ready to feed into the batch
// pipeline.
func FetchTicketsByLabel(labels, projects []string) ([]TicketInput, error) {
	return fetchTicketsByLabel(labels, projects, execClaudeLabelQuery)
}

func fetchTicketsByLabel(labels, projects []string, runner LabelQueryRunner) ([]TicketInput, error) {
	labels = cleanList(labels)
	projects = cleanList(projects)
	if len(labels) == 0 {
		return nil, fmt.Errorf("at least one label is required")
	}

	jql := buildJQL(labels, projects)
	prompt := buildLabelQueryPrompt(jql)

	stdout, err := runner(prompt)
	if err != nil {
		return nil, fmt.Errorf("claude query failed: %w", err)
	}

	return parseTicketsOutput(stdout, jql)
}

// cleanList trims whitespace and drops empty entries.
func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// buildJQL composes a JQL query from labels and optional projects.
//
// Multiple labels are combined with AND — a ticket must carry all of them
// to match. This lets callers narrow results with e.g. a routing label
// (`--jira-label for-agents,leap_one_server` returns tickets that have
// BOTH labels, not either).
//
// Projects are combined with OR ("in any of these projects"), which is
// the common scoping case. Values are quoted to tolerate spaces,
// hyphens, and other benign chars.
func buildJQL(labels, projects []string) string {
	var parts []string
	if len(projects) > 0 {
		parts = append(parts, fmt.Sprintf("project in (%s)", quoteJoin(projects)))
	}
	for _, label := range labels {
		parts = append(parts, fmt.Sprintf("labels = %s", strconv.Quote(label)))
	}
	return strings.Join(parts, " AND ")
}

func quoteJoin(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	return strings.Join(quoted, ",")
}

// buildLabelQueryPrompt returns the prompt text clade sends to Claude.
// The contract is intentionally rigid: Claude must emit exactly one JSON
// line that clade can parse — nothing else, no prose, no code fences.
func buildLabelQueryPrompt(jql string) string {
	return fmt.Sprintf(`You have access to the Atlassian Jira MCP tools.

Run searchJiraIssuesUsingJql with this JQL:
  %s

OUTPUT RULES — follow them exactly:
- Output a single line of JSON, and NOTHING else.
- No prose. No explanation. No markdown code fences. No leading or trailing lines.
- On success, the line must match this shape:
    {"tickets":["PROJ-123","PROJ-124"],"error":null}
- On zero matches (query succeeded, no results):
    {"tickets":[],"error":null}
- On any failure (MCP unavailable, auth error, invalid JQL):
    {"tickets":[],"error":"<brief reason>"}
- Only include ticket keys (e.g. PROJ-123), not URLs, summaries, or other fields.`, jql)
}

// parseTicketsOutput scans Claude's stdout for a line that matches the
// labelQueryResponse contract. It tolerates prose before/after the JSON
// line; if line-by-line scanning fails, it falls back to extracting the
// outermost balanced JSON object from the whole buffer (handles the case
// where Claude pretty-prints the JSON over multiple lines).
func parseTicketsOutput(stdout []byte, jql string) ([]TicketInput, error) {
	resp, ok := findResponseLine(stdout)
	if !ok {
		resp, ok = findResponseInBuffer(stdout)
	}
	if !ok {
		return nil, fmt.Errorf(
			"could not parse claude response (expected JSON with 'tickets' field)\nJQL: %s\nstdout: %s",
			jql,
			truncateForError(string(stdout), 500),
		)
	}

	if resp.Error != nil && *resp.Error != "" {
		return nil, fmt.Errorf("jira query failed: %s\nJQL: %s", *resp.Error, jql)
	}

	if len(resp.Tickets) == 0 {
		return nil, fmt.Errorf("no tickets matched JQL: %s", jql)
	}

	inputs := make([]TicketInput, 0, len(resp.Tickets))
	for _, id := range resp.Tickets {
		id = strings.TrimSpace(id)
		if id != "" {
			inputs = append(inputs, TicketInput{ID: id})
		}
	}

	if len(inputs) == 0 {
		return nil, fmt.Errorf("no valid ticket IDs in response\nJQL: %s", jql)
	}

	return inputs, nil
}

// findResponseLine walks stdout line-by-line and returns the first line
// that parses as a labelQueryResponse carrying one of our fields.
func findResponseLine(stdout []byte) (*labelQueryResponse, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
			continue
		}
		if resp, ok := tryUnmarshalResponse([]byte(line)); ok {
			return resp, true
		}
	}
	return nil, false
}

// findResponseInBuffer is a fallback for pretty-printed JSON that spans
// multiple lines. It extracts the first balanced {...} object from the
// buffer and tries to unmarshal it.
func findResponseInBuffer(stdout []byte) (*labelQueryResponse, bool) {
	start := bytes.IndexByte(stdout, '{')
	for start != -1 {
		end := matchBalancedBrace(stdout, start)
		if end == -1 {
			return nil, false
		}
		if resp, ok := tryUnmarshalResponse(stdout[start : end+1]); ok {
			return resp, true
		}
		// Not our shape — skip past this object and keep looking.
		next := bytes.IndexByte(stdout[end+1:], '{')
		if next == -1 {
			return nil, false
		}
		start = end + 1 + next
	}
	return nil, false
}

// matchBalancedBrace returns the index of the '}' that closes the '{' at
// openIdx, or -1 if unbalanced. String literals are respected so braces
// inside JSON strings don't confuse the count.
func matchBalancedBrace(buf []byte, openIdx int) int {
	depth := 0
	inString := false
	escaped := false
	for i := openIdx; i < len(buf); i++ {
		c := buf[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// tryUnmarshalResponse attempts to parse raw as a labelQueryResponse and
// requires at least one of our fields to be present (so we don't accept
// unrelated JSON that happens to live in stdout).
func tryUnmarshalResponse(raw []byte) (*labelQueryResponse, bool) {
	var candidate labelQueryResponse
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return nil, false
	}
	if candidate.Tickets == nil && candidate.Error == nil {
		return nil, false
	}
	return &candidate, true
}

func truncateForError(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func execClaudeLabelQuery(prompt string) ([]byte, error) {
	cmd := exec.Command("claude", "-p", prompt, "--permission-mode", "bypassPermissions")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
