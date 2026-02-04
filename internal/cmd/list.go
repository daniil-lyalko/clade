package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/daniil-lyalko/pacer/internal/config"
	"github.com/daniil-lyalko/pacer/internal/git"
	"github.com/daniil-lyalko/pacer/internal/ui"
	"github.com/spf13/cobra"
)

var listJSONFlag bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all active worktrees, projects, and scratches",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVar(&listJSONFlag, "json", false, "Output as JSON")
}

// ListWorktreeJSON is the JSON output structure for a worktree
type ListWorktreeJSON struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Branch   string `json:"branch"`
	Path     string `json:"path"`
	Status   string `json:"status"` // "tracked", "untracked", "orphaned"
	Stale    bool   `json:"stale,omitempty"`
	LastUsed string `json:"last_used,omitempty"`
}

// ListProjectJSON is the JSON output structure for a project
type ListProjectJSON struct {
	Name     string   `json:"name"`
	Branch   string   `json:"branch"`
	Path     string   `json:"path"`
	Repos    []string `json:"repos"`
	LastUsed string   `json:"last_used"`
}

// ListScratchJSON is the JSON output structure for a scratch
type ListScratchJSON struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Ticket   string `json:"ticket,omitempty"`
	Stale    bool   `json:"stale,omitempty"`
	LastUsed string `json:"last_used"`
}

// ListOutputJSON is the JSON output structure for list command
type ListOutputJSON struct {
	Worktrees map[string][]ListWorktreeJSON `json:"worktrees"`
	Projects  []ListProjectJSON             `json:"projects,omitempty"`
	Scratches []ListScratchJSON             `json:"scratches,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// JSON output
	if listJSONFlag {
		return runListJSON(cfg, state)
	}

	hasContent := false

	// List worktrees (grouped by repo) - includes both tracked and untracked
	if hasWorktrees := printAllWorktrees(cfg, state); hasWorktrees {
		hasContent = true
	}

	// List v1 experiments (legacy format)
	if len(state.Experiments) > 0 {
		hasContent = true
		ui.Header("Experiments (legacy):")
		for _, exp := range state.Experiments {
			printExperiment(exp)
		}
		fmt.Println()
	}

	// List projects
	if len(state.Projects) > 0 {
		hasContent = true
		ui.Header("Projects")
		for _, proj := range state.Projects {
			printProject(proj)
		}
		fmt.Println()
	}

	// List scratches
	if len(state.Scratches) > 0 {
		hasContent = true
		ui.Header("Scratch")
		for _, scratch := range state.Scratches {
			printScratch(scratch)
		}
		fmt.Println()
	}

	if !hasContent {
		ui.Info("No active worktrees, projects, or scratch folders")
		ui.Detail("Create one with: pacer <name>")
		ui.Detail("Or with type prefix: pacer <name> -t feature")
		ui.Detail("Or for no-git: pacer scratch <name>")
	}

	return nil
}

// runListJSON outputs list data as JSON
func runListJSON(cfg *config.Config, state *config.State) error {
	output := ListOutputJSON{
		Worktrees: make(map[string][]ListWorktreeJSON),
	}

	// Collect all repos
	repoSet := make(map[string]string) // name -> path
	for name, path := range cfg.Repos {
		repoSet[name] = config.ExpandPath(path)
	}

	// Process each repo
	for repoName, repoPath := range repoSet {
		gitWorktrees, err := git.ListWorktreesDetailed(repoPath)
		if err != nil {
			continue
		}

		trackedWorktrees := state.Worktrees[repoName]
		if trackedWorktrees == nil {
			trackedWorktrees = make(map[string]*config.Worktree)
		}

		seenTracked := make(map[string]bool)
		var repoWorktrees []ListWorktreeJSON

		// Git worktrees
		for _, gwt := range gitWorktrees {
			if gwt.IsMain {
				continue
			}

			var matched *config.Worktree
			var matchedName string
			for name, wt := range trackedWorktrees {
				wtPath := config.GetWorktreePath(cfg, repoName, wt)
				if wtPath == gwt.Path {
					matched = wt
					matchedName = name
					seenTracked[name] = true
					break
				}
			}

			if matched != nil {
				stale := time.Since(matched.LastUsed) > 7*24*time.Hour
				repoWorktrees = append(repoWorktrees, ListWorktreeJSON{
					Name:     matchedName,
					Label:    matched.Label,
					Branch:   matched.Branch,
					Path:     gwt.Path,
					Status:   "tracked",
					Stale:    stale,
					LastUsed: matched.LastUsed.Format(time.RFC3339),
				})
			} else {
				folderName := filepath.Base(gwt.Path)
				repoWorktrees = append(repoWorktrees, ListWorktreeJSON{
					Name:   folderName,
					Branch: gwt.Branch,
					Path:   gwt.Path,
					Status: "untracked",
				})
			}
		}

		// Orphaned worktrees
		for name, wt := range trackedWorktrees {
			if seenTracked[name] {
				continue
			}
			wtPath := config.GetWorktreePath(cfg, repoName, wt)
			repoWorktrees = append(repoWorktrees, ListWorktreeJSON{
				Name:     name,
				Label:    wt.Label,
				Branch:   wt.Branch,
				Path:     wtPath,
				Status:   "orphaned",
				LastUsed: wt.LastUsed.Format(time.RFC3339),
			})
		}

		if len(repoWorktrees) > 0 {
			output.Worktrees[repoName] = repoWorktrees
		}
	}

	// Projects
	for _, proj := range state.Projects {
		var repoNames []string
		for _, r := range proj.Repos {
			repoNames = append(repoNames, r.Name)
		}
		output.Projects = append(output.Projects, ListProjectJSON{
			Name:     proj.Name,
			Branch:   proj.Branch,
			Path:     proj.Path,
			Repos:    repoNames,
			LastUsed: proj.LastUsed.Format(time.RFC3339),
		})
	}

	// Scratches
	for _, scratch := range state.Scratches {
		stale := time.Since(scratch.LastUsed) > 7*24*time.Hour
		output.Scratches = append(output.Scratches, ListScratchJSON{
			Name:     scratch.Name,
			Path:     scratch.Path,
			Ticket:   scratch.Ticket,
			Stale:    stale,
			LastUsed: scratch.LastUsed.Format(time.RFC3339),
		})
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// UntrackedWorktree represents a git worktree not tracked by pacer
type UntrackedWorktree struct {
	Path   string
	Branch string
}

// printAllWorktrees prints all worktrees (tracked + untracked) grouped by repo
func printAllWorktrees(cfg *config.Config, state *config.State) bool {
	// Collect all repos: registered + any with tracked worktrees
	repoSet := make(map[string]string) // name -> path

	// Add registered repos
	for name, path := range cfg.Repos {
		repoSet[name] = config.ExpandPath(path)
	}

	// Add repos from state that might not be registered anymore
	for repoName := range state.Worktrees {
		if _, ok := repoSet[repoName]; !ok {
			// Try to find the path from existing worktrees
			for _, wt := range state.Worktrees[repoName] {
				if wt.Path != "" {
					// Can't easily get repo path from worktree path
					// Just skip - user should re-register
					break
				}
			}
		}
	}

	if len(repoSet) == 0 && len(state.Worktrees) == 0 {
		return false
	}

	// Sort repo names
	var repoNames []string
	for name := range repoSet {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)

	hasAny := false

	for _, repoName := range repoNames {
		repoPath := repoSet[repoName]

		// Get actual git worktrees
		gitWorktrees, err := git.ListWorktreesDetailed(repoPath)
		if err != nil {
			// Can't query this repo, skip
			continue
		}

		// Get tracked worktrees from state
		trackedWorktrees := state.Worktrees[repoName]
		if trackedWorktrees == nil {
			trackedWorktrees = make(map[string]*config.Worktree)
		}

		// Build list of worktrees to display
		type displayWorktree struct {
			Name      string
			Tracked   *config.Worktree
			Untracked *UntrackedWorktree
			Path      string
			Orphaned  bool // tracked but git worktree gone
		}

		var toDisplay []displayWorktree
		seenTracked := make(map[string]bool)

		// First, check git worktrees
		for _, gwt := range gitWorktrees {
			if gwt.IsMain {
				continue // Skip main repo
			}

			// Try to match with tracked worktree
			var matched *config.Worktree
			var matchedName string
			for name, wt := range trackedWorktrees {
				wtPath := config.GetWorktreePath(cfg, repoName, wt)
				if wtPath == gwt.Path {
					matched = wt
					matchedName = name
					seenTracked[name] = true
					break
				}
			}

			if matched != nil {
				toDisplay = append(toDisplay, displayWorktree{
					Name:    matchedName,
					Tracked: matched,
					Path:    gwt.Path,
				})
			} else {
				// Untracked - use folder name as display name
				folderName := filepath.Base(gwt.Path)
				toDisplay = append(toDisplay, displayWorktree{
					Name: folderName,
					Untracked: &UntrackedWorktree{
						Path:   gwt.Path,
						Branch: gwt.Branch,
					},
					Path: gwt.Path,
				})
			}
		}

		// Check for orphaned tracked worktrees (in state but not in git)
		for name, wt := range trackedWorktrees {
			if seenTracked[name] {
				continue
			}
			wtPath := config.GetWorktreePath(cfg, repoName, wt)
			toDisplay = append(toDisplay, displayWorktree{
				Name:     name,
				Tracked:  wt,
				Path:     wtPath,
				Orphaned: true,
			})
		}

		if len(toDisplay) == 0 {
			continue
		}

		hasAny = true

		// Print repo header
		fmt.Printf("%s (%d worktree", ui.Cyan(repoName), len(toDisplay))
		if len(toDisplay) != 1 {
			fmt.Print("s")
		}
		fmt.Println(")")

		// Sort: tracked by last_used, then untracked alphabetically
		sort.Slice(toDisplay, func(i, j int) bool {
			// Tracked first
			if toDisplay[i].Tracked != nil && toDisplay[j].Tracked == nil {
				return true
			}
			if toDisplay[i].Tracked == nil && toDisplay[j].Tracked != nil {
				return false
			}
			// Both tracked: by last used
			if toDisplay[i].Tracked != nil && toDisplay[j].Tracked != nil {
				return toDisplay[i].Tracked.LastUsed.After(toDisplay[j].Tracked.LastUsed)
			}
			// Both untracked: alphabetically
			return toDisplay[i].Name < toDisplay[j].Name
		})

		// Print each worktree
		for _, dw := range toDisplay {
			if dw.Orphaned {
				// Tracked but git worktree is gone
				fmt.Printf("  %-16s %-12s %s\n",
					dw.Name,
					ui.Dim(dw.Tracked.Label),
					ui.Red("(orphaned)"),
				)
			} else if dw.Tracked != nil {
				printWorktreeCompact(cfg, repoName, dw.Tracked)
			} else {
				// Untracked
				fmt.Printf("  %-16s %-12s %s\n",
					dw.Name,
					ui.Dim("(untracked)"),
					ui.Dim(dw.Untracked.Branch),
				)
			}
		}
		fmt.Println()
	}

	// Also print tracked worktrees from repos not in repoSet (legacy/missing repos)
	for repoName, worktrees := range state.Worktrees {
		if _, inSet := repoSet[repoName]; inSet {
			continue // Already handled above
		}
		if len(worktrees) == 0 {
			continue
		}

		hasAny = true
		fmt.Printf("%s %s (%d worktree", ui.Cyan(repoName), ui.Yellow("(unregistered repo)"), len(worktrees))
		if len(worktrees) != 1 {
			fmt.Print("s")
		}
		fmt.Println(")")

		for _, wt := range worktrees {
			printWorktreeCompact(cfg, repoName, wt)
		}
		fmt.Println()
	}

	return hasAny
}

// printWorktreeCompact prints a worktree in compact single-line format
func printWorktreeCompact(cfg *config.Config, repoName string, wt *config.Worktree) {
	wtPath := config.GetWorktreePath(cfg, repoName, wt)
	age := formatAgeShort(wt.LastUsed)

	// Check if stale (older than 7 days)
	staleMarker := ""
	if time.Since(wt.LastUsed) > 7*24*time.Hour {
		staleMarker = ui.Yellow("(stale)")
	}

	// Check for uncommitted changes
	statusMarker := ""
	if hasChanges, _ := git.HasUncommittedChanges(wtPath); hasChanges {
		statusMarker = ui.Yellow("*")
	}

	// Format: "  name          label         age  (stale)"
	fmt.Printf("  %-16s %-12s %s %s%s\n",
		wt.Name+statusMarker,
		ui.Dim(wt.Label),
		ui.Dim(age),
		staleMarker,
		"",
	)
}

func printExperiment(exp *config.Experiment) {
	repoName := filepath.Base(exp.Repo)
	age := formatAge(exp.LastUsed)

	// Check status
	status := ""
	if hasChanges, _ := git.HasUncommittedChanges(exp.Path); hasChanges {
		status = ui.Yellow("uncommitted changes")
	} else {
		status = ui.Green("clean")
	}

	// Check if stale (older than 7 days)
	staleMarker := ""
	if time.Since(exp.LastUsed) > 7*24*time.Hour {
		staleMarker = " " + ui.Yellow("(stale)")
	}

	fmt.Printf("  %s %s%s\n", ui.Cyan(exp.Name), ui.Dim("("+repoName+")"), staleMarker)
	ui.KeyValue("Branch", exp.Branch)
	ui.KeyValue("Path", exp.Path)
	ui.KeyValue("Age", age)
	ui.KeyValue("Status", status)
	if exp.Ticket != "" {
		ui.KeyValue("Ticket", exp.Ticket)
	}
	fmt.Println()
}

func printProject(proj *config.Project) {
	age := formatAge(proj.LastUsed)

	var repoNames []string
	for _, r := range proj.Repos {
		repoNames = append(repoNames, r.Name)
	}

	fmt.Printf("  %s\n", ui.Cyan(proj.Name))
	ui.KeyValue("Branch", proj.Branch)
	ui.KeyValue("Path", proj.Path)
	ui.KeyValue("Repos", fmt.Sprintf("%v", repoNames))
	ui.KeyValue("Age", age)
	fmt.Println()
}

func printScratch(scratch *config.Scratch) {
	age := formatAge(scratch.LastUsed)

	// Check if stale (older than 7 days)
	staleMarker := ""
	if time.Since(scratch.LastUsed) > 7*24*time.Hour {
		staleMarker = " " + ui.Yellow("⚠")
	}

	fmt.Printf("  %s %s%s\n", ui.Cyan(scratch.Name), ui.Dim("(no-git)"), staleMarker)
	ui.KeyValue("Path", scratch.Path)
	ui.KeyValue("Age", age)
	if scratch.Ticket != "" {
		ui.KeyValue("Ticket", scratch.Ticket)
	}
	fmt.Println()
}

func formatAge(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

// formatAgeShort returns a compact age string like "2h" or "3d"
func formatAgeShort(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
