package batch

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/daniil-lyalko/clade/internal/ui"
)

// TicketStatus represents the current state of a ticket in the batch
type TicketStatus string

const (
	StatusQueued  TicketStatus = "queued"
	StatusRunning TicketStatus = "running"
	StatusDone    TicketStatus = "done"
	StatusFailed  TicketStatus = "failed"
	StatusBlocked TicketStatus = "blocked"
)

// TicketResult tracks a single ticket's progress
type TicketResult struct {
	ID        string       `json:"id"`
	Repos     []string     `json:"repos,omitempty"` // Repos this ticket spans
	Status    TicketStatus `json:"status"`
	Branch    string       `json:"branch,omitempty"`
	PR        string       `json:"pr,omitempty"`
	Error     string       `json:"error,omitempty"`
	StartedAt *time.Time   `json:"started_at,omitempty"`
	DoneAt    *time.Time   `json:"done_at,omitempty"`
	LogFile   string       `json:"log_file,omitempty"`
}

// TicketInput represents a parsed ticket from input (CLI or CSV)
type TicketInput struct {
	ID    string
	Repos []string // Repos this ticket needs (first is primary workdir, rest get --add-dir)
}

// Duration returns the elapsed time for this ticket
func (t *TicketResult) Duration() time.Duration {
	if t.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if t.DoneAt != nil {
		end = *t.DoneAt
	}
	return end.Sub(*t.StartedAt)
}

// BatchState persists the state of a batch run
type BatchState struct {
	ID          string          `json:"id"`
	CreatedAt   time.Time       `json:"created_at"`
	Repo        string          `json:"repo"`
	Concurrency int             `json:"concurrency"`
	MaxBudget   float64         `json:"max_budget_usd,omitempty"`
	Prompt      string          `json:"prompt_template,omitempty"`
	Tickets     []*TicketResult `json:"tickets"`

	mu      sync.Mutex `json:"-"`
	baseDir string     `json:"-"`

	// repoMu serializes worktree creation per source repo to avoid git lock conflicts
	repoMu   map[string]*sync.Mutex `json:"-"`
	repoMuMu sync.Mutex             `json:"-"`
}

// BatchDir returns the directory where batch state and logs are stored
func BatchDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "clade", "batches")
}

// NewBatch creates a new batch with the given tickets
func NewBatch(inputs []TicketInput, repo string, concurrency int, maxBudget float64, prompt string) *BatchState {
	id := time.Now().Format("2006-01-02-1504")
	tickets := make([]*TicketResult, len(inputs))
	for i, input := range inputs {
		tickets[i] = &TicketResult{
			ID:     input.ID,
			Repos:  input.Repos,
			Status: StatusQueued,
		}
	}

	return &BatchState{
		ID:          id,
		CreatedAt:   time.Now(),
		Repo:        repo,
		Concurrency: concurrency,
		MaxBudget:   maxBudget,
		Prompt:      prompt,
		Tickets:     tickets,
	}
}

// RepoMutex returns a mutex for the given repo path, creating one if needed.
// This serializes worktree creation per source repo to avoid git lock conflicts.
func (b *BatchState) RepoMutex(repoPath string) *sync.Mutex {
	b.repoMuMu.Lock()
	defer b.repoMuMu.Unlock()
	if b.repoMu == nil {
		b.repoMu = make(map[string]*sync.Mutex)
	}
	if _, ok := b.repoMu[repoPath]; !ok {
		b.repoMu[repoPath] = &sync.Mutex{}
	}
	return b.repoMu[repoPath]
}

// Dir returns the directory for this batch
func (b *BatchState) Dir() string {
	return filepath.Join(BatchDir(), b.ID)
}

// Save persists the batch state to disk
func (b *BatchState) Save() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	dir := b.Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "batch.json"), data, 0600)
}

// LoadBatch reads a batch state from disk
func LoadBatch(id string) (*BatchState, error) {
	path := filepath.Join(BatchDir(), id, "batch.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var batch BatchState
	if err := json.Unmarshal(data, &batch); err != nil {
		return nil, err
	}
	return &batch, nil
}

// ListBatches returns all batch IDs sorted by most recent
func ListBatches() ([]string, error) {
	dir := BatchDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ids []string
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.IsDir() {
			// Check that batch.json exists
			if _, err := os.Stat(filepath.Join(dir, e.Name(), "batch.json")); err == nil {
				ids = append(ids, e.Name())
			}
		}
	}
	return ids, nil
}

// UpdateTicket updates a ticket's status thread-safely
func (b *BatchState) UpdateTicket(id string, fn func(*TicketResult)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range b.Tickets {
		if t.ID == id {
			fn(t)
			return
		}
	}
}

// WorktreeResult holds the paths created for a ticket
type WorktreeResult struct {
	PrimaryPath string   // Main worktree path (workdir for claude)
	AddDirs     []string // Additional repo worktree paths (--add-dir)
	Branch      string   // Branch name
}

// Run executes the batch with the configured concurrency
func (b *BatchState) Run(repoPath string, createWorktreeFn func(ticket *TicketResult) (*WorktreeResult, error)) error {
	if err := b.Save(); err != nil {
		return fmt.Errorf("failed to save initial batch state: %w", err)
	}

	// Create log directory
	logDir := filepath.Join(b.Dir(), "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	ui.Header("Starting batch %s", b.ID)
	ui.KeyValue("Tickets", fmt.Sprintf("%d", len(b.Tickets)))
	ui.KeyValue("Concurrency", fmt.Sprintf("%d", b.Concurrency))
	ui.KeyValue("Repo", b.Repo)
	fmt.Println()

	// Build work queue
	work := make(chan *TicketResult, len(b.Tickets))
	for _, t := range b.Tickets {
		work <- t
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < b.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for ticket := range work {
				b.processTicket(ticket, logDir, createWorktreeFn)
			}
		}(i)
	}

	wg.Wait()

	// Final status
	b.Save()
	fmt.Println()
	b.PrintStatus()

	return nil
}

// processTicket handles a single ticket: create worktree(s), run claude, track result
func (b *BatchState) processTicket(ticket *TicketResult, logDir string, createWorktreeFn func(*TicketResult) (*WorktreeResult, error)) {
	now := time.Now()
	b.UpdateTicket(ticket.ID, func(t *TicketResult) {
		t.Status = StatusRunning
		t.StartedAt = &now
	})
	b.Save()

	ui.Info("[%s] Starting...", ticket.ID)

	// Step 1: Create worktree(s)
	wtResult, err := createWorktreeFn(ticket)
	if err != nil {
		b.markFailed(ticket.ID, fmt.Sprintf("worktree creation failed: %v", err))
		return
	}

	b.UpdateTicket(ticket.ID, func(t *TicketResult) {
		t.Branch = wtResult.Branch
	})
	b.Save()

	if len(wtResult.AddDirs) > 0 {
		ui.Info("[%s] Worktrees ready: %s + %d additional repos", ticket.ID, wtResult.PrimaryPath, len(wtResult.AddDirs))
	} else {
		ui.Info("[%s] Worktree ready at %s", ticket.ID, wtResult.PrimaryPath)
	}

	// Step 2: Run Claude Code in non-interactive mode
	logFile := filepath.Join(logDir, ticket.ID+".log")
	b.UpdateTicket(ticket.ID, func(t *TicketResult) {
		t.LogFile = logFile
	})

	prompt := b.buildPrompt(ticket.ID, wtResult.Branch)

	args := []string{
		"-p", prompt,
		"--permission-mode", "bypassPermissions",
	}

	// Add additional repo directories
	for _, dir := range wtResult.AddDirs {
		args = append(args, "--add-dir", dir)
	}

	if b.MaxBudget > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", b.MaxBudget))
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = wtResult.PrimaryPath

	// Capture output to log file
	logF, err := os.Create(logFile)
	if err != nil {
		b.markFailed(ticket.ID, fmt.Sprintf("failed to create log file: %v", err))
		return
	}
	defer logF.Close()

	cmd.Stdout = logF
	cmd.Stderr = logF

	ui.Info("[%s] Running claude...", ticket.ID)

	err = cmd.Run()
	doneAt := time.Now()

	if err != nil {
		b.markFailed(ticket.ID, fmt.Sprintf("claude exited with error: %v", err))
		return
	}

	// Check log for PR URL
	prURL := extractPRURL(logFile)

	b.UpdateTicket(ticket.ID, func(t *TicketResult) {
		t.Status = StatusDone
		t.DoneAt = &doneAt
		if prURL != "" {
			t.PR = prURL
		}
	})
	b.Save()

	duration := doneAt.Sub(*ticket.StartedAt).Round(time.Second)
	ui.Success("[%s] Done (%s)", ticket.ID, duration)
}

func (b *BatchState) markFailed(ticketID, errMsg string) {
	now := time.Now()
	b.UpdateTicket(ticketID, func(t *TicketResult) {
		t.Status = StatusFailed
		t.Error = errMsg
		t.DoneAt = &now
	})
	b.Save()
	ui.Error("[%s] Failed: %s", ticketID, errMsg)
}

func (b *BatchState) buildPrompt(ticketID, branch string) string {
	if b.Prompt != "" {
		// Replace placeholders in custom prompt
		prompt := strings.ReplaceAll(b.Prompt, "{TICKET_ID}", ticketID)
		prompt = strings.ReplaceAll(prompt, "{BRANCH}", branch)
		return prompt
	}

	// Find repos for this ticket to customize the prompt
	var ticketRepos []string
	for _, t := range b.Tickets {
		if t.ID == ticketID {
			ticketRepos = t.Repos
			break
		}
	}

	multiRepoNote := ""
	if len(ticketRepos) > 1 {
		multiRepoNote = fmt.Sprintf(`
Note: This ticket spans multiple repositories. You have access to all of them
via --add-dir. Make changes in each repo as needed. Commit and push the branch
"%s" in EACH repo that has changes. Create a PR for each repo that has changes.
`, branch)
	}

	return fmt.Sprintf(`You are working on Jira ticket %s.%s

Step 1 — Understand the ticket:
  Use the Jira MCP tools (getJiraIssue) to fetch %s.
  Read the title, description, acceptance criteria, and any linked tickets.
  A title alone is often enough if it clearly describes what to do
  (e.g., "Add phone validation to contact form" is actionable).

Step 2 — Evaluate if you can proceed:
  You need to be able to determine:
    - WHAT needs to change (which feature, behavior, or bug)
    - WHERE in the codebase it lives (search to find relevant files)
  If BOTH are clear, proceed to Step 3.
  If the ticket is truly ambiguous (you cannot determine what or where),
  go to Step 2b.

Step 2b — Blocked ticket:
  Add a comment to the Jira ticket using the MCP tools:
    "🤖 Automated: Unable to implement — this ticket needs more detail.
     Missing: [explain what is unclear or missing]
     Please update the ticket description and re-queue."
  Then STOP. Do not write any code. Exit immediately.

Step 3 — Implement:
  1. Analyze the codebase to understand the relevant code and patterns
  2. Implement the solution following existing code style and conventions
  3. Run any relevant tests (npm test, go test, etc.) and fix failures
  4. Commit your changes with a descriptive message referencing %s
  5. Push the branch: git push -u origin %s
  6. Create a PR: gh pr create --title "[%s] <brief description>" --body "<summary of changes>"
  7. Add a comment to the Jira ticket with the PR link

Important:
- Only push to branch %s — never to main/master
- Follow existing code style and patterns
- Prefer the simplest, most straightforward solution
- If tests exist for the area you changed, make sure they pass`, ticketID, multiRepoNote, ticketID, ticketID, branch, ticketID, branch)
}

// PrintStatus displays the current batch status
func (b *BatchState) PrintStatus() {
	ui.Header("Batch %s", b.ID)
	ui.KeyValue("Repo", b.Repo)
	ui.KeyValue("Workers", fmt.Sprintf("%d", b.Concurrency))

	var queued, running, done, failed, blocked int
	fmt.Println()
	for _, t := range b.Tickets {
		switch t.Status {
		case StatusQueued:
			queued++
			fmt.Printf("  %s %s  %s\n", ui.Dim("○"), ui.Cyan(t.ID), ui.Dim("queued"))
		case StatusRunning:
			running++
			elapsed := t.Duration().Round(time.Second)
			fmt.Printf("  %s %s  %s  %s\n", ui.Yellow("●"), ui.Cyan(t.ID), ui.Yellow("running"), ui.Dim(fmt.Sprintf("(%s elapsed)", elapsed)))
		case StatusDone:
			done++
			duration := t.Duration().Round(time.Second)
			prInfo := ""
			if t.PR != "" {
				prInfo = "  " + t.PR
			}
			fmt.Printf("  %s %s  %s  %s%s\n", ui.Green("✓"), ui.Cyan(t.ID), ui.Green("done"), ui.Dim(fmt.Sprintf("(%s)", duration)), prInfo)
		case StatusBlocked:
			blocked++
			errInfo := t.Error
			if errInfo == "" {
				errInfo = "needs more detail"
			}
			fmt.Printf("  %s %s  %s  %s\n", ui.Yellow("⚠"), ui.Cyan(t.ID), ui.Yellow("blocked"), ui.Dim(errInfo))
		case StatusFailed:
			failed++
			fmt.Printf("  %s %s  %s  %s\n", ui.Red("✗"), ui.Cyan(t.ID), ui.Red("failed"), ui.Dim(t.Error))
		}
	}

	fmt.Println()
	summary := fmt.Sprintf("%d done, %d running, %d queued", done, running, queued)
	if blocked > 0 {
		summary += fmt.Sprintf(", %d blocked", blocked)
	}
	if failed > 0 {
		summary += fmt.Sprintf(", %d failed", failed)
	}
	ui.Detail("Summary: %s", summary)
}

// extractPRURL scans a log file for a GitHub PR URL
func extractPRURL(logFile string) string {
	f, err := os.Open(logFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Look for github.com PR URLs
		if idx := strings.Index(line, "https://github.com/"); idx != -1 {
			url := line[idx:]
			// Trim to just the URL
			if end := strings.IndexAny(url, " \t\n\"'`])}"); end != -1 {
				url = url[:end]
			}
			if strings.Contains(url, "/pull/") {
				return url
			}
		}
	}
	return ""
}

// ParseTicketsFromCSV reads ticket IDs from a CSV file
// Supports:
//   - One ID per line (no header)
//   - Header row with "ticket"/"id"/"key" column and optional "repo" column
func ParseTicketsFromCSV(path string) ([]TicketInput, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1 // Allow variable fields
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("empty CSV file")
	}

	// Check if first row is a header
	ticketCol := 0
	reposCol := -1
	hasHeader := false
	firstRow := records[0]
	for i, col := range firstRow {
		lower := strings.ToLower(strings.TrimSpace(col))
		switch lower {
		case "ticket", "id", "key", "ticket_id":
			ticketCol = i
			hasHeader = true
		case "repo", "repos", "repository", "repositories":
			reposCol = i
			hasHeader = true
		}
	}

	var tickets []TicketInput
	startRow := 0
	if hasHeader {
		startRow = 1
	}

	for _, row := range records[startRow:] {
		if ticketCol >= len(row) {
			continue
		}
		id := strings.TrimSpace(row[ticketCol])
		if id == "" {
			continue
		}
		input := TicketInput{ID: id}
		if reposCol >= 0 && reposCol < len(row) {
			reposStr := strings.TrimSpace(row[reposCol])
			if reposStr != "" {
				for _, r := range strings.Split(reposStr, ",") {
					r = strings.TrimSpace(r)
					if r != "" {
						input.Repos = append(input.Repos, r)
					}
				}
			}
		}
		tickets = append(tickets, input)
	}

	if len(tickets) == 0 {
		return nil, fmt.Errorf("no ticket IDs found in CSV")
	}

	return tickets, nil
}

// ParseTicketsFromFile reads ticket IDs from a plain text file (one per line) or CSV
func ParseTicketsFromFile(path string) ([]TicketInput, error) {
	// Try CSV first
	if strings.HasSuffix(strings.ToLower(path), ".csv") {
		return ParseTicketsFromCSV(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tickets []TicketInput
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			tickets = append(tickets, TicketInput{ID: line})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(tickets) == 0 {
		return nil, fmt.Errorf("no ticket IDs found in file")
	}

	return tickets, nil
}
