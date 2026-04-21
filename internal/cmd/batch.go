package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/batch"
	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
)

var (
	batchFileFlag        string
	batchConcurrencyFlag int
	batchRepoFlag        string
	batchBudgetFlag      float64
	batchPromptFlag      string
	batchTypeFlag        string
	batchFromFlag        string
	batchDryRunFlag      bool
	batchProjectFlag     bool
	batchJiraLabelFlag   []string
	batchJiraProjectFlag []string
)

// labelTicketFetcher is the seam used by runBatch to resolve tickets from
// Jira labels. Production code points it at batch.FetchTicketsByLabel;
// tests swap it for a canned fetcher.
var labelTicketFetcher = batch.FetchTicketsByLabel

var batchCmd = &cobra.Command{
	Use:   "batch [ticket-ids...]",
	Short: "Run multiple tickets in parallel with autonomous Claude agents",
	Long: `Process multiple Jira tickets concurrently. Each ticket gets its own
worktree and an autonomous Claude Code agent that fetches the ticket,
implements the solution, and creates a PR.

Input methods:
  clade batch PROJ-123 PROJ-456 PROJ-789       # Direct ticket IDs
  clade batch --file tickets.csv                # From CSV file
  clade batch --file tickets.txt                # From text file (one per line)
  clade batch --jira-label bug                  # Fetched from Jira by label
  clade batch --jira-label bug --jira-project PROJ  # Scoped to projects

CSV files can have a header row with "ticket", "id", or "key" column,
or just one ticket ID per line.

Label-based fetching delegates to the Atlassian Jira MCP (no Jira
credentials are handled by clade directly). Inputs from all sources are
merged and deduplicated.

Examples:
  clade batch SALESPRO-1234 SALESPRO-1235 --concurrency 2
  clade batch --file sprint-tickets.csv --concurrency 3 --repo leap-360
  clade batch PROJ-100 --budget 5.00 --type bug
  clade batch PROJ-100 --prompt "Fix the bug described in {TICKET_ID}"
  clade batch --jira-label triage-bug --jira-project SALESPRO,LEAP --dry-run`,
	Args: cobra.ArbitraryArgs,
	RunE: runBatch,
}

var batchStatusCmd = &cobra.Command{
	Use:   "status [batch-id]",
	Short: "Show status of batch runs",
	Long: `Show the status of batch runs. Without arguments, lists all batches.
With a batch ID, shows detailed status for that batch.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBatchStatus,
}

var batchLogsCmd = &cobra.Command{
	Use:   "logs <batch-id> <ticket-id>",
	Short: "Show logs for a ticket in a batch",
	Args:  cobra.ExactArgs(2),
	RunE:  runBatchLogs,
}

func init() {
	rootCmd.AddCommand(batchCmd)
	batchCmd.AddCommand(batchStatusCmd)
	batchCmd.AddCommand(batchLogsCmd)

	batchCmd.Flags().StringVarP(&batchFileFlag, "file", "F", "", "CSV or text file with ticket IDs")
	batchCmd.Flags().IntVarP(&batchConcurrencyFlag, "concurrency", "n", 2, "Number of parallel workers")
	batchCmd.Flags().StringVarP(&batchRepoFlag, "repo", "r", "", "Repository to work in")
	batchCmd.Flags().Float64Var(&batchBudgetFlag, "budget", 0, "Max budget per ticket in USD (e.g., 5.00)")
	batchCmd.Flags().StringVar(&batchPromptFlag, "prompt", "", "Custom prompt template (use {TICKET_ID} and {BRANCH} placeholders)")
	batchCmd.Flags().StringVarP(&batchTypeFlag, "type", "t", "feature", "Branch type (feature, bug, spike, chore, hotfix, docs)")
	batchCmd.Flags().StringVarP(&batchFromFlag, "from", "f", "", "Base branch to create from (default: origin's default branch)")
	batchCmd.Flags().BoolVar(&batchDryRunFlag, "dry-run", false, "Preview what would be created without running")
	batchCmd.Flags().BoolVar(&batchProjectFlag, "project", false, "Create a clade project per ticket (multi-repo workspace)")
	batchCmd.Flags().StringSliceVar(&batchJiraLabelFlag, "jira-label", nil, "Comma-separated Jira labels to fetch tickets for (via Atlassian MCP)")
	batchCmd.Flags().StringSliceVar(&batchJiraProjectFlag, "jira-project", nil, "Comma-separated Jira project keys to scope label search (requires --jira-label)")
}

// labelFetchInfo captures what was requested from Jira so dry-run output
// can display it. nil when no label fetching happened.
type labelFetchInfo struct {
	Labels   []string
	Projects []string
	Resolved int
}

func runBatch(cmd *cobra.Command, args []string) error {
	// Validate Jira label-fetching flag composition up front.
	//
	// Cobra's StringSliceVar leaves the backing slice empty when the user
	// passes --jira-label="" — so slice length alone isn't enough to
	// distinguish "flag not passed" from "flag passed with empty value."
	// Flags().Changed reports whether the flag was passed regardless of
	// value. Tests call runBatch with a nil cmd and set the slice
	// directly, so we also accept a non-empty slice as evidence.
	labels := trimStrings(batchJiraLabelFlag)
	jiraProjects := trimStrings(batchJiraProjectFlag)

	labelExplicit := len(batchJiraLabelFlag) > 0 ||
		(cmd != nil && cmd.Flags().Changed("jira-label"))
	projectExplicit := len(batchJiraProjectFlag) > 0 ||
		(cmd != nil && cmd.Flags().Changed("jira-project"))

	if projectExplicit && !labelExplicit {
		return fmt.Errorf("--jira-project requires --jira-label")
	}
	if labelExplicit && len(labels) == 0 {
		return fmt.Errorf("--jira-label cannot be empty")
	}

	// Collect ticket inputs from args and/or file and/or Jira label query.
	var inputs []batch.TicketInput

	// From CLI args
	for _, arg := range args {
		inputs = append(inputs, batch.TicketInput{ID: arg})
	}

	// From file
	if batchFileFlag != "" {
		fileTickets, err := batch.ParseTicketsFromFile(batchFileFlag)
		if err != nil {
			return fmt.Errorf("failed to read tickets file: %w", err)
		}
		inputs = append(inputs, fileTickets...)
	}

	// From Jira label query (MCP-delegated; no Go-side Jira client)
	var fetchInfo *labelFetchInfo
	if len(labels) > 0 {
		ui.Info("Fetching tickets from Jira for labels: %s", strings.Join(labels, ", "))
		labelTickets, err := labelTicketFetcher(labels, jiraProjects)
		if err != nil {
			return fmt.Errorf("jira label fetch failed: %w", err)
		}
		fetchInfo = &labelFetchInfo{
			Labels:   labels,
			Projects: jiraProjects,
			Resolved: len(labelTickets),
		}
		inputs = append(inputs, labelTickets...)
	}

	if len(inputs) == 0 {
		return fmt.Errorf("no ticket IDs provided. Pass them as arguments, use --file, or use --jira-label")
	}

	// Deduplicate by ticket ID
	inputs = dedupInputs(inputs)

	// Resolve default repo
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	defaultRepoPath, err := resolveRepo(cfg, batchRepoFlag)
	if err != nil {
		return err
	}
	defaultRepoName := git.GetRepoName(defaultRepoPath)

	// Validate per-ticket repos if specified
	for i, input := range inputs {
		for j, repo := range input.Repos {
			resolved, err := resolveRepo(cfg, repo)
			if err != nil {
				return fmt.Errorf("ticket %s: invalid repo '%s': %w", input.ID, repo, err)
			}
			inputs[i].Repos[j] = resolved
		}
	}

	// Resolve label config
	labelCfg, ok := cfg.GetLabelConfig(batchTypeFlag)
	if !ok {
		return fmt.Errorf("unknown type '%s'. %s", batchTypeFlag, availableTypesHelp(cfg))
	}

	// Dry run
	if batchDryRunFlag {
		return printBatchDryRun(inputs, defaultRepoName, defaultRepoPath, labelCfg.BranchPrefix, cfg, fetchInfo)
	}

	// Create batch
	b := batch.NewBatch(inputs, defaultRepoName, batchConcurrencyFlag, batchBudgetFlag, batchPromptFlag)

	var createWorktreeFn func(ticket *batch.TicketResult) (*batch.WorktreeResult, error)

	if batchProjectFlag {
		// Project mode: create a clade project per ticket
		createWorktreeFn = func(ticket *batch.TicketResult) (*batch.WorktreeResult, error) {
			return createBatchProject(ticket, b, cfg, defaultRepoPath, labelCfg.BranchPrefix)
		}
	} else {
		// Worktree mode: create individual worktrees per repo
		createWorktreeFn = func(ticket *batch.TicketResult) (*batch.WorktreeResult, error) {
			return createBatchWorktrees(ticket, b, cfg, defaultRepoPath, labelCfg.BranchPrefix)
		}
	}

	return b.Run(defaultRepoPath, createWorktreeFn)
}

// createBatchWorktrees creates individual worktrees per repo for a ticket (original behavior)
func createBatchWorktrees(ticket *batch.TicketResult, b *batch.BatchState, cfg *config.Config, defaultRepoPath, branchPrefix string) (*batch.WorktreeResult, error) {
	ticketID := ticket.ID

	// Determine repos for this ticket
	repos := ticket.Repos
	if len(repos) == 0 {
		repos = []string{defaultRepoPath}
	}

	// Build branch name
	var branch string
	if branchPrefix != "" {
		branch = branchPrefix + "/" + ticketID
	} else {
		branch = ticketID
	}

	result := &batch.WorktreeResult{Branch: branch}

	// Create a worktree in each repo (serialized per source repo to avoid git lock conflicts)
	for i, repoPath := range repos {
		repoName := git.GetRepoName(repoPath)

		// Lock per source repo to prevent concurrent git operations
		mu := b.RepoMutex(repoPath)
		mu.Lock()

		// Check if worktree already exists
		state, err := config.LoadState(cfg)
		if err != nil {
			mu.Unlock()
			return nil, err
		}

		var wtPath string
		if existing := state.GetWorktree(repoName, ticketID); existing != nil {
			mu.Unlock()
			ui.Info("[%s] Worktree for %s already exists, reusing", ticketID, repoName)
			wtPath = config.GetWorktreePath(cfg, repoName, existing)
		} else {
			// Create worktree programmatically (no agent, no editor)
			err = CreateWorktree(ticketID, WorktreeConfig{
				Label:        batchTypeFlag,
				BranchPrefix: branchPrefix,
			}, WorktreeOptions{
				RepoFlag:     repoPath,
				FromFlag:     batchFromFlag,
				NoAgentFlag:  true,
				NoEditorFlag: true,
			})
			mu.Unlock()
			if err != nil {
				return nil, fmt.Errorf("repo %s: %w", repoName, err)
			}
			wtPath = config.WorktreePath(cfg, repoName, ticketID)
		}

		if i == 0 {
			result.PrimaryPath = wtPath
		} else {
			result.AddDirs = append(result.AddDirs, wtPath)
		}
	}

	return result, nil
}

// createBatchProject creates a clade project per ticket, grouping all repos under one directory
func createBatchProject(ticket *batch.TicketResult, b *batch.BatchState, cfg *config.Config, defaultRepoPath, branchPrefix string) (*batch.WorktreeResult, error) {
	ticketID := ticket.ID

	// Determine repos for this ticket
	repos := ticket.Repos
	if len(repos) == 0 {
		repos = []string{defaultRepoPath}
	}

	// Build branch name
	var branch string
	if branchPrefix != "" {
		branch = branchPrefix + "/" + ticketID
	} else {
		branch = ticketID
	}

	// Check if project already exists
	state, err := config.LoadState(cfg)
	if err != nil {
		return nil, err
	}

	if existing, ok := state.Projects[ticketID]; ok {
		ui.Info("[%s] Project already exists, reusing", ticketID)
		result := &batch.WorktreeResult{Branch: existing.Branch}
		if len(existing.Repos) > 0 {
			result.PrimaryPath = filepath.Join(existing.Path, existing.Repos[0].Name)
			for i := 1; i < len(existing.Repos); i++ {
				result.AddDirs = append(result.AddDirs, filepath.Join(existing.Path, existing.Repos[i].Name))
			}
		}
		return result, nil
	}

	// Create project directory
	projectPath := filepath.Join(cfg.ProjectsDir(), ticketID)
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create project directory: %w", err)
	}

	// Preflight check all repos
	branchResults := git.PreflightCheck(repos, branch)

	// Create worktrees for each repo
	var createdRepos []config.ProjectRepo
	result := &batch.WorktreeResult{Branch: branch}

	for i, repoPath := range repos {
		repoName := git.GetRepoName(repoPath)
		folderName := filepath.Base(repoPath)
		worktreePath := filepath.Join(projectPath, folderName)
		info := branchResults[repoPath]

		// Lock per source repo to prevent concurrent git operations
		mu := b.RepoMutex(repoPath)
		mu.Lock()

		ui.Info("[%s] Creating %s...", ticketID, folderName)

		var wtErr error
		switch info.Status {
		case git.BranchNotFound:
			_, wtErr = git.CreateWorktreeNew(repoPath, worktreePath, branch, "")
		case git.BranchLocalOnly, git.BranchBoth:
			wtErr = git.CreateWorktreeFromBranch(repoPath, worktreePath, branch)
		case git.BranchRemoteOnly:
			wtErr = git.CreateWorktreeTrackRemote(repoPath, worktreePath, branch)
		}

		mu.Unlock()

		if wtErr != nil {
			// Clean up partial project on failure
			for _, r := range createdRepos {
				os.RemoveAll(filepath.Join(projectPath, r.Name))
			}
			os.Remove(projectPath)
			return nil, fmt.Errorf("repo %s: %w", repoName, wtErr)
		}

		// Copy gitignored files (.env, .npmrc, etc.)
		if err := copyGitignoredFilesForProject(cfg, repoPath, worktreePath); err != nil {
			ui.Warn("[%s] Failed to copy some files for %s: %v", ticketID, folderName, err)
		}

		createdRepos = append(createdRepos, config.ProjectRepo{
			Name:   folderName,
			Source: repoPath,
		})

		if i == 0 {
			result.PrimaryPath = worktreePath
		} else {
			result.AddDirs = append(result.AddDirs, worktreePath)
		}
	}

	// Write .clade-project.json
	projectMeta := map[string]interface{}{
		"type":    "project",
		"name":    ticketID,
		"branch":  branch,
		"repos":   createdRepos,
		"created": time.Now().Format(time.RFC3339),
	}
	metaPath := filepath.Join(projectPath, ".clade-project.json")
	if err := writeProjectJSON(metaPath, projectMeta); err != nil {
		ui.Warn("[%s] Failed to write .clade-project.json: %v", ticketID, err)
	}

	// Update state
	project := &config.Project{
		Name:     ticketID,
		Path:     projectPath,
		Branch:   branch,
		Repos:    createdRepos,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}
	state.Projects[ticketID] = project
	if err := state.Save(cfg); err != nil {
		ui.Warn("[%s] Failed to save state: %v", ticketID, err)
	}

	return result, nil
}

func runBatchStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		// Show specific batch
		b, err := batch.LoadBatch(args[0])
		if err != nil {
			return fmt.Errorf("failed to load batch '%s': %w", args[0], err)
		}
		b.PrintStatus()
		return nil
	}

	// List all batches
	ids, err := batch.ListBatches()
	if err != nil {
		return fmt.Errorf("failed to list batches: %w", err)
	}

	if len(ids) == 0 {
		ui.Info("No batch runs found")
		return nil
	}

	ui.Header("Batch runs")
	for _, id := range ids {
		b, err := batch.LoadBatch(id)
		if err != nil {
			ui.Warn("Failed to load batch %s: %v", id, err)
			continue
		}

		var done, failed, blocked, total int
		total = len(b.Tickets)
		for _, t := range b.Tickets {
			switch t.Status {
			case batch.StatusDone:
				done++
			case batch.StatusFailed:
				failed++
			case batch.StatusBlocked:
				blocked++
			}
		}

		status := ui.Green("complete")
		if failed > 0 || blocked > 0 {
			parts := fmt.Sprintf("%d/%d done", done, total)
			if blocked > 0 {
				parts += fmt.Sprintf(", %d blocked", blocked)
			}
			if failed > 0 {
				parts += fmt.Sprintf(", %d failed", failed)
			}
			status = ui.Yellow(parts)
		} else if done < total {
			status = ui.Cyan(fmt.Sprintf("%d/%d done", done, total))
		}

		fmt.Printf("  %s  %s  %s  %s\n",
			ui.Cyan(id),
			ui.Dim("("+b.Repo+")"),
			status,
			ui.Dim(fmt.Sprintf("%d tickets", total)),
		)
	}

	return nil
}

func runBatchLogs(cmd *cobra.Command, args []string) error {
	batchID := args[0]
	ticketID := args[1]

	b, err := batch.LoadBatch(batchID)
	if err != nil {
		return fmt.Errorf("failed to load batch '%s': %w", batchID, err)
	}

	// Find the ticket
	var logFile string
	for _, t := range b.Tickets {
		if t.ID == ticketID {
			logFile = t.LogFile
			break
		}
	}

	if logFile == "" {
		logFile = filepath.Join(batch.BatchDir(), batchID, "logs", ticketID+".log")
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		return fmt.Errorf("no logs found for %s in batch %s", ticketID, batchID)
	}

	fmt.Print(string(data))
	return nil
}

func printBatchDryRun(inputs []batch.TicketInput, defaultRepoName, defaultRepoPath, branchPrefix string, cfg *config.Config, fetchInfo *labelFetchInfo) error {
	fmt.Println()
	fmt.Println(ui.Bold("Dry run - no changes will be made"))
	fmt.Println()

	ui.KeyValue("Default repo", fmt.Sprintf("%s (%s)", defaultRepoName, defaultRepoPath))
	ui.KeyValue("Tickets", fmt.Sprintf("%d", len(inputs)))
	ui.KeyValue("Concurrency", fmt.Sprintf("%d", batchConcurrencyFlag))
	if fetchInfo != nil {
		ui.KeyValue("Jira labels", strings.Join(fetchInfo.Labels, ", "))
		if len(fetchInfo.Projects) > 0 {
			ui.KeyValue("Jira projects", strings.Join(fetchInfo.Projects, ", "))
		}
		ui.KeyValue("Resolved from labels", fmt.Sprintf("%d", fetchInfo.Resolved))
	}
	if batchProjectFlag {
		ui.KeyValue("Mode", "project (multi-repo workspace per ticket)")
	}
	if batchBudgetFlag > 0 {
		ui.KeyValue("Budget per ticket", fmt.Sprintf("$%.2f", batchBudgetFlag))
	}

	fmt.Println()
	fmt.Println(ui.Dim("Would create:"))
	for _, input := range inputs {
		var branch string
		if branchPrefix != "" {
			branch = branchPrefix + "/" + input.ID
		} else {
			branch = input.ID
		}
		if len(input.Repos) > 1 {
			var repoNames []string
			for _, r := range input.Repos {
				repoNames = append(repoNames, git.GetRepoName(r))
			}
			fmt.Printf("  %s → branch %s %s\n", ui.Cyan(input.ID), ui.Dim(branch), ui.Dim(fmt.Sprintf("(%s)", strings.Join(repoNames, " + "))))
		} else if len(input.Repos) == 1 {
			fmt.Printf("  %s → branch %s %s\n", ui.Cyan(input.ID), ui.Dim(branch), ui.Dim(fmt.Sprintf("(%s)", git.GetRepoName(input.Repos[0]))))
		} else {
			fmt.Printf("  %s → branch %s\n", ui.Cyan(input.ID), ui.Dim(branch))
		}
	}

	fmt.Println()
	fmt.Println(ui.Dim("Each ticket will run:"))
	fmt.Printf("  claude -p <prompt> --permission-mode bypassPermissions\n")
	if batchBudgetFlag > 0 {
		fmt.Printf("  --max-budget-usd %.2f\n", batchBudgetFlag)
	}

	fmt.Println()
	fmt.Println(ui.Dim("Remove --dry-run to execute"))
	return nil
}

// trimStrings returns a copy of values with whitespace trimmed and empty
// entries removed.
func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func dedupInputs(inputs []batch.TicketInput) []batch.TicketInput {
	seen := make(map[string]bool)
	var result []batch.TicketInput
	for _, input := range inputs {
		key := strings.TrimSpace(input.ID)
		if key != "" && !seen[key] {
			seen[key] = true
			input.ID = key
			result = append(result, input)
		}
	}
	return result
}
