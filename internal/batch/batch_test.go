package batch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBatch(t *testing.T) {
	inputs := []TicketInput{
		{ID: "PROJ-1"},
		{ID: "PROJ-2", Repos: []string{"/repo1", "/repo2"}},
	}

	b := NewBatch(inputs, "my-repo", 3, 5.0, "")

	assert.NotEmpty(t, b.ID)
	assert.Equal(t, "my-repo", b.Repo)
	assert.Equal(t, 3, b.Concurrency)
	assert.Equal(t, 5.0, b.MaxBudget)
	assert.Len(t, b.Tickets, 2)

	assert.Equal(t, "PROJ-1", b.Tickets[0].ID)
	assert.Equal(t, StatusQueued, b.Tickets[0].Status)
	assert.Empty(t, b.Tickets[0].Repos)

	assert.Equal(t, "PROJ-2", b.Tickets[1].ID)
	assert.Equal(t, []string{"/repo1", "/repo2"}, b.Tickets[1].Repos)
}

func TestBatchSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	// Override BatchDir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	inputs := []TicketInput{{ID: "PROJ-100"}}
	b := NewBatch(inputs, "test-repo", 2, 0, "custom prompt")
	b.ID = "test-batch-001"

	err := b.Save()
	require.NoError(t, err)

	// Verify file exists
	batchFile := filepath.Join(tmpDir, ".config", "clade", "batches", "test-batch-001", "batch.json")
	_, err = os.Stat(batchFile)
	require.NoError(t, err)

	// Load it back
	loaded, err := LoadBatch("test-batch-001")
	require.NoError(t, err)

	assert.Equal(t, "test-batch-001", loaded.ID)
	assert.Equal(t, "test-repo", loaded.Repo)
	assert.Equal(t, 2, loaded.Concurrency)
	assert.Equal(t, "custom prompt", loaded.Prompt)
	assert.Len(t, loaded.Tickets, 1)
	assert.Equal(t, "PROJ-100", loaded.Tickets[0].ID)
	assert.Equal(t, StatusQueued, loaded.Tickets[0].Status)
}

func TestListBatches(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// No batches yet
	ids, err := ListBatches()
	require.NoError(t, err)
	assert.Empty(t, ids)

	// Create two batches
	b1 := NewBatch([]TicketInput{{ID: "T-1"}}, "repo", 1, 0, "")
	b1.ID = "2026-01-01-0900"
	require.NoError(t, b1.Save())

	b2 := NewBatch([]TicketInput{{ID: "T-2"}}, "repo", 1, 0, "")
	b2.ID = "2026-01-02-0900"
	require.NoError(t, b2.Save())

	ids, err = ListBatches()
	require.NoError(t, err)
	assert.Len(t, ids, 2)
	// Most recent first
	assert.Equal(t, "2026-01-02-0900", ids[0])
	assert.Equal(t, "2026-01-01-0900", ids[1])
}

func TestUpdateTicket(t *testing.T) {
	b := NewBatch([]TicketInput{{ID: "T-1"}, {ID: "T-2"}}, "repo", 1, 0, "")

	b.UpdateTicket("T-1", func(tr *TicketResult) {
		tr.Status = StatusRunning
		now := time.Now()
		tr.StartedAt = &now
	})

	assert.Equal(t, StatusRunning, b.Tickets[0].Status)
	assert.NotNil(t, b.Tickets[0].StartedAt)
	// T-2 unchanged
	assert.Equal(t, StatusQueued, b.Tickets[1].Status)
}

func TestUpdateTicket_NonExistent(t *testing.T) {
	b := NewBatch([]TicketInput{{ID: "T-1"}}, "repo", 1, 0, "")

	// Should not panic
	b.UpdateTicket("T-999", func(tr *TicketResult) {
		tr.Status = StatusDone
	})

	assert.Equal(t, StatusQueued, b.Tickets[0].Status)
}

func TestTicketResult_Duration(t *testing.T) {
	// Not started
	tr := &TicketResult{ID: "T-1", Status: StatusQueued}
	assert.Equal(t, time.Duration(0), tr.Duration())

	// Running
	start := time.Now().Add(-5 * time.Second)
	tr.StartedAt = &start
	tr.Status = StatusRunning
	assert.InDelta(t, 5*time.Second, tr.Duration(), float64(500*time.Millisecond))

	// Done
	end := start.Add(10 * time.Second)
	tr.DoneAt = &end
	tr.Status = StatusDone
	assert.Equal(t, 10*time.Second, tr.Duration())
}

func TestBuildPrompt_Default(t *testing.T) {
	b := NewBatch([]TicketInput{{ID: "PROJ-1"}}, "repo", 1, 0, "")

	prompt := b.buildPrompt("PROJ-1", "feat/PROJ-1")

	assert.Contains(t, prompt, "PROJ-1")
	assert.Contains(t, prompt, "feat/PROJ-1")
	assert.Contains(t, prompt, "Step 1")
	assert.Contains(t, prompt, "getJiraIssue")
}

func TestBuildPrompt_MultiRepo(t *testing.T) {
	b := NewBatch([]TicketInput{{ID: "PROJ-1", Repos: []string{"/repo1", "/repo2"}}}, "repo", 1, 0, "")

	prompt := b.buildPrompt("PROJ-1", "feat/PROJ-1")

	assert.Contains(t, prompt, "multiple repositories")
	assert.Contains(t, prompt, "--add-dir")
	assert.Contains(t, prompt, "EACH repo")
}

func TestBuildPrompt_Custom(t *testing.T) {
	b := NewBatch([]TicketInput{{ID: "PROJ-1"}}, "repo", 1, 0, "Fix {TICKET_ID} on branch {BRANCH} now")

	prompt := b.buildPrompt("PROJ-1", "feat/PROJ-1")

	assert.Equal(t, "Fix PROJ-1 on branch feat/PROJ-1 now", prompt)
}

func TestBuildPrompt_CustomIgnoresDefault(t *testing.T) {
	b := NewBatch([]TicketInput{{ID: "PROJ-1", Repos: []string{"/r1", "/r2"}}}, "repo", 1, 0, "Just do {TICKET_ID}")

	prompt := b.buildPrompt("PROJ-1", "feat/PROJ-1")

	// Custom prompt should NOT include multi-repo note or default steps
	assert.Equal(t, "Just do PROJ-1", prompt)
	assert.NotContains(t, prompt, "multiple repositories")
	assert.NotContains(t, prompt, "Step 1")
}

func TestParseTicketsFromCSV_WithHeader(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "tickets.csv")

	content := `ticket,repos
PROJ-1,"repo-a,repo-b"
PROJ-2,repo-a
PROJ-3,
`
	require.NoError(t, os.WriteFile(csvPath, []byte(content), 0644))

	tickets, err := ParseTicketsFromCSV(csvPath)
	require.NoError(t, err)

	assert.Len(t, tickets, 3)

	assert.Equal(t, "PROJ-1", tickets[0].ID)
	assert.Equal(t, []string{"repo-a", "repo-b"}, tickets[0].Repos)

	assert.Equal(t, "PROJ-2", tickets[1].ID)
	assert.Equal(t, []string{"repo-a"}, tickets[1].Repos)

	assert.Equal(t, "PROJ-3", tickets[2].ID)
	assert.Empty(t, tickets[2].Repos)
}

func TestParseTicketsFromCSV_NoHeader(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "tickets.csv")

	content := `PROJ-1
PROJ-2
PROJ-3
`
	require.NoError(t, os.WriteFile(csvPath, []byte(content), 0644))

	tickets, err := ParseTicketsFromCSV(csvPath)
	require.NoError(t, err)

	assert.Len(t, tickets, 3)
	assert.Equal(t, "PROJ-1", tickets[0].ID)
	assert.Empty(t, tickets[0].Repos)
}

func TestParseTicketsFromCSV_AlternateHeaders(t *testing.T) {
	tests := []struct {
		name    string
		header  string
	}{
		{"id header", "id,repository"},
		{"key header", "key,repos"},
		{"ticket_id header", "ticket_id,repositories"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			csvPath := filepath.Join(tmpDir, "tickets.csv")

			content := tt.header + "\nPROJ-1,repo-a\n"
			require.NoError(t, os.WriteFile(csvPath, []byte(content), 0644))

			tickets, err := ParseTicketsFromCSV(csvPath)
			require.NoError(t, err)

			assert.Len(t, tickets, 1)
			assert.Equal(t, "PROJ-1", tickets[0].ID)
			assert.Equal(t, []string{"repo-a"}, tickets[0].Repos)
		})
	}
}

func TestParseTicketsFromCSV_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "tickets.csv")

	require.NoError(t, os.WriteFile(csvPath, []byte(""), 0644))

	_, err := ParseTicketsFromCSV(csvPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty CSV")
}

func TestParseTicketsFromCSV_HeaderOnly(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "tickets.csv")

	require.NoError(t, os.WriteFile(csvPath, []byte("ticket,repos\n"), 0644))

	_, err := ParseTicketsFromCSV(csvPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no ticket IDs")
}

func TestParseTicketsFromFile_TextFile(t *testing.T) {
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "tickets.txt")

	content := `PROJ-1
# this is a comment
PROJ-2

PROJ-3
`
	require.NoError(t, os.WriteFile(txtPath, []byte(content), 0644))

	tickets, err := ParseTicketsFromFile(txtPath)
	require.NoError(t, err)

	assert.Len(t, tickets, 3)
	assert.Equal(t, "PROJ-1", tickets[0].ID)
	assert.Equal(t, "PROJ-2", tickets[1].ID)
	assert.Equal(t, "PROJ-3", tickets[2].ID)
}

func TestParseTicketsFromFile_CSVExtension(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "tickets.csv")

	content := "ticket\nPROJ-1\nPROJ-2\n"
	require.NoError(t, os.WriteFile(csvPath, []byte(content), 0644))

	tickets, err := ParseTicketsFromFile(csvPath)
	require.NoError(t, err)

	assert.Len(t, tickets, 2)
}

func TestParseTicketsFromFile_NotFound(t *testing.T) {
	_, err := ParseTicketsFromFile("/nonexistent/file.txt")
	assert.Error(t, err)
}

func TestExtractPRURL(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "PR URL in output",
			content:  "Creating PR...\nhttps://github.com/org/repo/pull/123\nDone!",
			expected: "https://github.com/org/repo/pull/123",
		},
		{
			name:     "PR URL with trailing space",
			content:  "PR: https://github.com/org/repo/pull/456 created",
			expected: "https://github.com/org/repo/pull/456",
		},
		{
			name:     "No PR URL",
			content:  "Just some output\nNo PR here",
			expected: "",
		},
		{
			name:     "GitHub URL but not a PR",
			content:  "See https://github.com/org/repo/issues/789",
			expected: "",
		},
		{
			name:     "PR URL in quotes",
			content:  `Created "https://github.com/org/repo/pull/42" successfully`,
			expected: "https://github.com/org/repo/pull/42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logFile := filepath.Join(tmpDir, tt.name+".log")
			require.NoError(t, os.WriteFile(logFile, []byte(tt.content), 0644))

			result := extractPRURL(logFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPRURL_FileNotFound(t *testing.T) {
	result := extractPRURL("/nonexistent/file.log")
	assert.Empty(t, result)
}

func TestRepoMutex(t *testing.T) {
	b := NewBatch([]TicketInput{{ID: "T-1"}}, "repo", 1, 0, "")

	mu1 := b.RepoMutex("/repo/a")
	mu2 := b.RepoMutex("/repo/b")
	mu1Again := b.RepoMutex("/repo/a")

	// Same repo returns same mutex
	assert.Same(t, mu1, mu1Again)
	// Different repos return different mutexes
	assert.NotSame(t, mu1, mu2)
}

func TestRepoMutex_ConcurrentAccess(t *testing.T) {
	b := NewBatch([]TicketInput{{ID: "T-1"}}, "repo", 1, 0, "")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			repo := "/repo/a"
			if n%2 == 0 {
				repo = "/repo/b"
			}
			mu := b.RepoMutex(repo)
			mu.Lock()
			mu.Unlock()
		}(i)
	}
	wg.Wait()
}

func TestRun_WithMockClaude(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	inputs := []TicketInput{{ID: "T-1"}, {ID: "T-2"}}
	b := NewBatch(inputs, "test-repo", 2, 0, "")

	createFn := func(ticket *TicketResult) (*WorktreeResult, error) {
		wtDir := filepath.Join(tmpDir, "worktrees", ticket.ID)
		os.MkdirAll(wtDir, 0755)
		return &WorktreeResult{
			PrimaryPath: wtDir,
			Branch:      "feat/" + ticket.ID,
		}, nil
	}

	// This will fail because "claude" binary doesn't exist in test,
	// but it validates the run loop, state management, and concurrency
	err := b.Run(tmpDir, createFn)
	assert.NoError(t, err)

	// Both tickets should have been processed (failed since no claude binary)
	for _, ticket := range b.Tickets {
		assert.Equal(t, StatusFailed, ticket.Status)
		assert.NotNil(t, ticket.StartedAt)
		assert.NotNil(t, ticket.DoneAt)
		assert.Contains(t, ticket.Error, "claude exited with error")
	}

	// Batch state should have been saved
	loaded, err := LoadBatch(b.ID)
	require.NoError(t, err)
	assert.Len(t, loaded.Tickets, 2)
}

func TestBatchDir(t *testing.T) {
	dir := BatchDir()
	assert.Contains(t, dir, ".config/clade/batches")
}
