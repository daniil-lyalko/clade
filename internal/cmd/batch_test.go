package cmd

import (
	"fmt"
	"testing"

	"github.com/daniil-lyalko/clade/internal/batch"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrimStrings(t *testing.T) {
	got := trimStrings([]string{" bug ", "", "urgent", "   "})
	assert.Equal(t, []string{"bug", "urgent"}, got)

	// Nil input returns empty slice.
	assert.Empty(t, trimStrings(nil))
}

// resetBatchFlags clears package-level batch flag state so tests don't
// leak into each other. Also clears cobra's per-flag Changed bit on
// batchCmd so tests that drive the flag parser don't influence the next.
func resetBatchFlags() {
	batchFileFlag = ""
	batchJiraLabelFlag = nil
	batchJiraProjectFlag = nil
	labelTicketFetcher = batch.FetchTicketsByLabel

	batchCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
}

func TestRunBatch_JiraProjectWithoutLabel(t *testing.T) {
	resetBatchFlags()
	defer resetBatchFlags()

	batchJiraProjectFlag = []string{"PROJ"}

	err := runBatch(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--jira-project requires --jira-label")
}

func TestRunBatch_EmptyJiraLabel(t *testing.T) {
	resetBatchFlags()
	defer resetBatchFlags()

	// All entries whitespace-only → post-trim, labels are empty.
	batchJiraLabelFlag = []string{"", "   "}

	err := runBatch(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--jira-label cannot be empty")
}

func TestRunBatch_LabelFetchErrorSurfaces(t *testing.T) {
	resetBatchFlags()
	defer resetBatchFlags()

	batchJiraLabelFlag = []string{"bug"}
	labelTicketFetcher = func(labels, projects []string) ([]batch.TicketInput, error) {
		assert.Equal(t, []string{"bug"}, labels)
		assert.Empty(t, projects)
		return nil, fmt.Errorf("mcp unavailable")
	}

	err := runBatch(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jira label fetch failed")
	assert.Contains(t, err.Error(), "mcp unavailable")
}

func TestRunBatch_LabelFetcherReceivesTrimmedValues(t *testing.T) {
	resetBatchFlags()
	defer resetBatchFlags()

	batchJiraLabelFlag = []string{"  bug  ", "urgent"}
	batchJiraProjectFlag = []string{"PROJ ", " OTHER"}
	called := false
	labelTicketFetcher = func(labels, projects []string) ([]batch.TicketInput, error) {
		called = true
		assert.Equal(t, []string{"bug", "urgent"}, labels)
		assert.Equal(t, []string{"PROJ", "OTHER"}, projects)
		return nil, fmt.Errorf("stop here") // short-circuit the rest
	}

	_ = runBatch(nil, nil)
	assert.True(t, called, "fetcher must be invoked with trimmed labels/projects")
}

// TestRunBatch_EmptyJiraLabelViaCobra drives the cobra flag parser directly
// to exercise the real user-facing path: `clade batch --jira-label ""`.
// Cobra leaves the backing slice empty in this case, so the check relies
// on Flags().Changed rather than slice length.
func TestRunBatch_EmptyJiraLabelViaCobra(t *testing.T) {
	resetBatchFlags()
	defer resetBatchFlags()

	err := batchCmd.ParseFlags([]string{"--jira-label", ""})
	require.NoError(t, err)

	err = runBatch(batchCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--jira-label cannot be empty")
}

func TestRunBatch_JiraProjectWithoutLabelViaCobra(t *testing.T) {
	resetBatchFlags()
	defer resetBatchFlags()

	err := batchCmd.ParseFlags([]string{"--jira-project", "PROJ"})
	require.NoError(t, err)

	err = runBatch(batchCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--jira-project requires --jira-label")
}
