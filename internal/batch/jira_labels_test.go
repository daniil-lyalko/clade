package batch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildJQL(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		projects []string
		want     string
	}{
		{
			name:   "single label, no project",
			labels: []string{"bug"},
			want:   `labels = "bug"`,
		},
		{
			name:     "single label, single project",
			labels:   []string{"bug"},
			projects: []string{"PROJ"},
			want:     `project in ("PROJ") AND labels = "bug"`,
		},
		{
			name:     "multiple labels use AND semantics",
			labels:   []string{"for-agents", "leap_one_server"},
			projects: []string{"LEAP"},
			want:     `project in ("LEAP") AND labels = "for-agents" AND labels = "leap_one_server"`,
		},
		{
			name:     "multiple labels and multiple projects",
			labels:   []string{"bug", "urgent"},
			projects: []string{"PROJ", "OTHER"},
			want:     `project in ("PROJ","OTHER") AND labels = "bug" AND labels = "urgent"`,
		},
		{
			name:   "label with space is quoted",
			labels: []string{"needs design"},
			want:   `labels = "needs design"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildJQL(tc.labels, tc.projects)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCleanList(t *testing.T) {
	got := cleanList([]string{" bug ", "", "urgent", "   "})
	assert.Equal(t, []string{"bug", "urgent"}, got)
}

func TestBuildLabelQueryPrompt(t *testing.T) {
	prompt := buildLabelQueryPrompt(`labels = "bug"`)
	assert.Contains(t, prompt, "searchJiraIssuesUsingJql")
	assert.Contains(t, prompt, `labels = "bug"`)
	assert.Contains(t, prompt, `{"tickets":`)
	assert.Contains(t, prompt, "OUTPUT RULES")
}

func TestParseTicketsOutput_CleanJSON(t *testing.T) {
	out := []byte(`{"tickets":["PROJ-1","PROJ-2"],"error":null}`)
	got, err := parseTicketsOutput(out, "test jql")
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}, {ID: "PROJ-2"}}, got)
}

func TestParseTicketsOutput_JSONWithProse(t *testing.T) {
	out := []byte(`Sure! Here is the result:
{"tickets":["PROJ-1"],"error":null}
Let me know if you need more.`)
	got, err := parseTicketsOutput(out, "test jql")
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}}, got)
}

func TestParseTicketsOutput_JSONInCodeFence(t *testing.T) {
	out := []byte("```json\n" +
		`{"tickets":["PROJ-1","PROJ-2"],"error":null}` + "\n" +
		"```")
	got, err := parseTicketsOutput(out, "test jql")
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}, {ID: "PROJ-2"}}, got)
}

func TestParseTicketsOutput_MultilineJSON(t *testing.T) {
	// Claude pretty-prints over multiple lines — line-by-line scan fails,
	// fallback extracts the balanced {...} object.
	out := []byte(`Here you go:
{
  "tickets": ["PROJ-1", "PROJ-2"],
  "error": null
}`)
	got, err := parseTicketsOutput(out, "test jql")
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}, {ID: "PROJ-2"}}, got)
}

func TestParseTicketsOutput_ErrorField(t *testing.T) {
	out := []byte(`{"tickets":[],"error":"MCP not configured"}`)
	_, err := parseTicketsOutput(out, `labels = "bug"`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jira query failed")
	assert.Contains(t, err.Error(), "MCP not configured")
	assert.Contains(t, err.Error(), `labels = "bug"`)
}

func TestParseTicketsOutput_ZeroMatches(t *testing.T) {
	out := []byte(`{"tickets":[],"error":null}`)
	_, err := parseTicketsOutput(out, `labels = "foo"`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tickets matched")
	assert.Contains(t, err.Error(), `labels = "foo"`)
}

func TestParseTicketsOutput_NoJSON(t *testing.T) {
	out := []byte(`I am sorry, I cannot help with that.`)
	_, err := parseTicketsOutput(out, "test jql")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not parse")
	assert.Contains(t, err.Error(), "I am sorry")
}

func TestParseTicketsOutput_IgnoresIrrelevantJSON(t *testing.T) {
	// An unrelated JSON object appears first; our response comes later.
	out := []byte(`{"status":"ok","count":3}
{"tickets":["PROJ-1"],"error":null}`)
	got, err := parseTicketsOutput(out, "test jql")
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}}, got)
}

func TestParseTicketsOutput_TrimsIDs(t *testing.T) {
	out := []byte(`{"tickets":["  PROJ-1  ","PROJ-2"],"error":null}`)
	got, err := parseTicketsOutput(out, "test jql")
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}, {ID: "PROJ-2"}}, got)
}

func TestParseTicketsOutput_AllEmptyIDs(t *testing.T) {
	out := []byte(`{"tickets":["","   "],"error":null}`)
	_, err := parseTicketsOutput(out, "test jql")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid ticket IDs")
}

func TestParseTicketsOutput_ErrorOnlyFieldWithoutTickets(t *testing.T) {
	// Claude returns just the error field, no tickets key at all.
	out := []byte(`{"error":"something broke"}`)
	_, err := parseTicketsOutput(out, "test jql")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "something broke")
}

func TestParseTicketsOutput_LargeBuffer(t *testing.T) {
	// Build a buffer well larger than bufio's default 64KB to verify the
	// scanner buffer handles it.
	var sb strings.Builder
	for i := 0; i < 5000; i++ {
		sb.WriteString("this is a filler line of prose from claude's explanation\n")
	}
	sb.WriteString(`{"tickets":["PROJ-1"],"error":null}` + "\n")
	got, err := parseTicketsOutput([]byte(sb.String()), "test jql")
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}}, got)
}

func TestFetchTicketsByLabel_RunnerHappy(t *testing.T) {
	runner := func(prompt string) ([]byte, error) {
		assert.Contains(t, prompt, `labels = "bug"`)
		return []byte(`{"tickets":["PROJ-1","PROJ-2"],"error":null}`), nil
	}
	got, err := fetchTicketsByLabel([]string{"bug"}, nil, runner)
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}, {ID: "PROJ-2"}}, got)
}

func TestFetchTicketsByLabel_RunnerHappyWithProjects(t *testing.T) {
	runner := func(prompt string) ([]byte, error) {
		assert.Contains(t, prompt, `project in ("PROJ") AND labels = "bug"`)
		return []byte(`{"tickets":["PROJ-1"],"error":null}`), nil
	}
	got, err := fetchTicketsByLabel([]string{"bug"}, []string{"PROJ"}, runner)
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "PROJ-1"}}, got)
}

func TestFetchTicketsByLabel_MultipleLabelsUseAND(t *testing.T) {
	// Verifies the repo-routing-via-label workflow:
	//   clade batch --jira-label for-agents,leap_one_server -r leap_one_server
	// Must AND the labels so only tickets with BOTH labels are returned.
	runner := func(prompt string) ([]byte, error) {
		assert.Contains(t, prompt, `labels = "for-agents" AND labels = "leap_one_server"`)
		return []byte(`{"tickets":["LEAP-1"],"error":null}`), nil
	}
	got, err := fetchTicketsByLabel([]string{"for-agents", "leap_one_server"}, nil, runner)
	require.NoError(t, err)
	assert.Equal(t, []TicketInput{{ID: "LEAP-1"}}, got)
}

func TestFetchTicketsByLabel_RunnerError(t *testing.T) {
	runner := func(prompt string) ([]byte, error) {
		return nil, fmt.Errorf("claude not found on PATH")
	}
	_, err := fetchTicketsByLabel([]string{"bug"}, nil, runner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude query failed")
	assert.Contains(t, err.Error(), "claude not found on PATH")
}

func TestFetchTicketsByLabel_EmptyLabels(t *testing.T) {
	called := false
	runner := func(prompt string) ([]byte, error) {
		called = true
		return nil, nil
	}
	_, err := fetchTicketsByLabel(nil, nil, runner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one label is required")
	assert.False(t, called, "runner must not be invoked when labels are empty")
}

func TestFetchTicketsByLabel_WhitespaceOnlyLabels(t *testing.T) {
	// Whitespace-only labels should be treated as empty after cleaning.
	runner := func(prompt string) ([]byte, error) {
		return nil, nil
	}
	_, err := fetchTicketsByLabel([]string{"", "   "}, nil, runner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one label is required")
}

func TestFetchTicketsByLabel_ZeroMatches(t *testing.T) {
	runner := func(prompt string) ([]byte, error) {
		return []byte(`{"tickets":[],"error":null}`), nil
	}
	_, err := fetchTicketsByLabel([]string{"bug"}, nil, runner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tickets matched")
}

func TestMatchBalancedBrace(t *testing.T) {
	tests := []struct {
		name string
		buf  string
		open int
		want int
	}{
		{"simple", `{"a":1}`, 0, 6},
		{"nested", `{"a":{"b":1}}`, 0, 12},
		{"brace in string", `{"a":"{}"}`, 0, 9},
		{"escaped quote in string", `{"a":"\""}`, 0, 9},
		{"unbalanced", `{"a":1`, 0, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchBalancedBrace([]byte(tc.buf), tc.open)
			assert.Equal(t, tc.want, got)
		})
	}
}
