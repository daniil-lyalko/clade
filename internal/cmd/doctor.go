package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/pacer/internal/config"
	"github.com/daniil-lyalko/pacer/internal/git"
	"github.com/daniil-lyalko/pacer/internal/ui"
	"github.com/spf13/cobra"
)

var (
	doctorFixFlag  bool
	doctorJSONFlag bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose common configuration issues",
	Long: `Run diagnostic checks to identify common configuration issues.

Checks performed:
  - Config file exists and is valid
  - State file exists and is valid
  - Base directory exists and is writable
  - Git is available
  - Configured agent is available
  - Registered repos exist and are git repos
  - Trust registry is readable

Use --fix to attempt automatic fixes for issues that can be repaired.
Use --json for machine-readable output.`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVar(&doctorFixFlag, "fix", false, "Attempt to fix issues automatically")
	doctorCmd.Flags().BoolVar(&doctorJSONFlag, "json", false, "Output as JSON")
}

// checkResult represents a single diagnostic check result
type checkResult struct {
	name    string
	ok      bool
	warning bool // true if it's a warning, false if it's a failure
	message string
	fixFunc func() error // function to fix the issue, nil if not fixable
}

// DoctorCheckJSON is the JSON output structure for a single check
type DoctorCheckJSON struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "ok", "warning", "failure"
	Message string `json:"message,omitempty"`
	Fixable bool   `json:"fixable"`
}

// DoctorOutputJSON is the JSON output structure for doctor command
type DoctorOutputJSON struct {
	Passed   int               `json:"passed"`
	Warnings int               `json:"warnings"`
	Failures int               `json:"failures"`
	Checks   []DoctorCheckJSON `json:"checks"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	var results []checkResult
	var warnings, failures, passed int

	// Check config file
	results = append(results, checkConfig())

	// Check state file
	results = append(results, checkState())

	// Check state location (legacy vs new path)
	results = append(results, checkStateLocation())

	// Check base directory
	results = append(results, checkBaseDir())

	// Check git
	results = append(results, checkGit())

	// Check agent
	results = append(results, checkAgent())

	// Check registered repos
	results = append(results, checkRepos()...)

	// Check trust registry
	results = append(results, checkTrustRegistry())

	// Global hooks checks
	results = append(results, checkGlobalHooks()...)

	// Consistency checks - state vs filesystem vs git
	results = append(results, checkOrphanedWorktrees()...)
	results = append(results, checkUntrackedWorktrees()...)
	results = append(results, checkPrunableWorktrees()...)

	// Count results
	for _, r := range results {
		if r.ok {
			passed++
		} else if r.warning {
			warnings++
		} else {
			failures++
		}
	}

	// JSON output
	if doctorJSONFlag {
		output := DoctorOutputJSON{
			Passed:   passed,
			Warnings: warnings,
			Failures: failures,
			Checks:   make([]DoctorCheckJSON, 0, len(results)),
		}

		for _, r := range results {
			status := "ok"
			if !r.ok && r.warning {
				status = "warning"
			} else if !r.ok {
				status = "failure"
			}

			output.Checks = append(output.Checks, DoctorCheckJSON{
				Name:    r.name,
				Status:  status,
				Message: r.message,
				Fixable: r.fixFunc != nil,
			})
		}

		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))

		if failures > 0 {
			return fmt.Errorf("doctor found issues")
		}
		return nil
	}

	// Human-readable output
	fmt.Println()
	fmt.Println(ui.Bold("Pacer Doctor"))
	fmt.Println()

	// Print results
	for _, r := range results {
		if r.ok {
			fmt.Printf("  %s %s\n", ui.Green("✓"), r.name)
			if r.message != "" {
				fmt.Printf("    %s\n", ui.Dim(r.message))
			}
		} else if r.warning {
			fmt.Printf("  %s %s\n", ui.Yellow("⚠"), r.name)
			if r.message != "" {
				fmt.Printf("    %s\n", ui.Dim(r.message))
			}
		} else {
			marker := ui.Red("✗")
			if r.fixFunc != nil {
				marker = ui.Red("✗") + ui.Dim(" (fixable)")
			}
			fmt.Printf("  %s %s\n", marker, r.name)
			if r.message != "" {
				fmt.Printf("    %s\n", ui.Dim(r.message))
			}
		}
	}

	fmt.Println()

	// Attempt fixes if --fix flag is set
	if doctorFixFlag {
		var fixable []checkResult
		for _, r := range results {
			if !r.ok && r.fixFunc != nil {
				fixable = append(fixable, r)
			}
		}

		if len(fixable) > 0 {
			fmt.Println(ui.Bold("Attempting fixes..."))
			fmt.Println()

			for _, r := range fixable {
				if err := r.fixFunc(); err != nil {
					ui.Error("Could not fix %s: %v", r.name, err)
				} else {
					ui.Success("Fixed: %s", r.name)
					failures-- // Decrement failure count for fixed issues
				}
			}
			fmt.Println()
		}
	}

	// Summary
	if failures > 0 {
		ui.Error("%d check(s) failed", failures)
		if !doctorFixFlag {
			ui.Info("Run 'pacer doctor --fix' to attempt automatic fixes")
		}
		return fmt.Errorf("doctor found issues")
	} else if warnings > 0 {
		ui.Warn("All checks passed with %d warning(s)", warnings)
	} else {
		ui.Success("All checks passed!")
	}

	return nil
}

func checkConfig() checkResult {
	configPath, err := config.ConfigPath()
	if err != nil {
		return checkResult{
			name:    "Config file",
			ok:      false,
			message: fmt.Sprintf("Failed to determine path: %v", err),
		}
	}

	// Check if config file exists
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		return checkResult{
			name:    "Config file",
			ok:      false,
			message: fmt.Sprintf("File not found: %s", configPath),
			fixFunc: func() error {
				cfg := config.DefaultConfig()
				return cfg.Save()
			},
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return checkResult{
			name:    "Config file",
			ok:      false,
			message: fmt.Sprintf("Failed to load: %v", err),
		}
	}

	details := ""
	if cfg.Agent != "" {
		details = fmt.Sprintf("agent: %s", cfg.Agent)
	}
	if cfg.Editor != "" {
		if details != "" {
			details += ", "
		}
		details += fmt.Sprintf("editor: %s", cfg.Editor)
	}
	if details == "" {
		details = "no agent or editor configured"
	}

	return checkResult{
		name:    fmt.Sprintf("Config file (%s)", filepath.Base(configPath)),
		ok:      true,
		message: details,
	}
}

func checkState() checkResult {
	cfg, err := config.Load()
	if err != nil {
		return checkResult{
			name:    "State file",
			ok:      false,
			message: fmt.Sprintf("Could not load config: %v", err),
		}
	}

	statePath := config.StatePath()

	// Check if state file exists
	if _, statErr := os.Stat(statePath); os.IsNotExist(statErr) {
		return checkResult{
			name:    "State file",
			ok:      false,
			message: fmt.Sprintf("File not found: %s", statePath),
			fixFunc: func() error {
				state := config.NewState()
				return state.Save(cfg)
			},
		}
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return checkResult{
			name:    "State file",
			ok:      false,
			message: fmt.Sprintf("Failed to load: %v", err),
		}
	}

	// Count items
	wtCount := 0
	for _, repos := range state.Worktrees {
		wtCount += len(repos)
	}

	details := fmt.Sprintf("v%d, %d worktree(s), %d project(s), %d scratch(es)",
		state.Version, wtCount, len(state.Projects), len(state.Scratches))

	return checkResult{
		name:    "State file",
		ok:      true,
		message: details,
	}
}

func checkBaseDir() checkResult {
	cfg, err := config.Load()
	if err != nil {
		return checkResult{
			name:    "Base directory",
			ok:      false,
			message: fmt.Sprintf("Could not load config: %v", err),
		}
	}

	baseDir := cfg.GetBaseDir()

	// Check if it exists
	info, err := os.Stat(baseDir)
	if os.IsNotExist(err) {
		// This is OK - it will be created on first use
		// But we can offer to create it now with --fix
		return checkResult{
			name:    "Base directory",
			ok:      true,
			warning: false,
			message: fmt.Sprintf("%s (will be created on first use)", baseDir),
			fixFunc: func() error {
				return os.MkdirAll(baseDir, 0755)
			},
		}
	}
	if err != nil {
		return checkResult{
			name:    "Base directory",
			ok:      false,
			message: fmt.Sprintf("Error checking %s: %v", baseDir, err),
		}
	}

	if !info.IsDir() {
		return checkResult{
			name:    "Base directory",
			ok:      false,
			message: fmt.Sprintf("%s exists but is not a directory", baseDir),
		}
	}

	// Check writable by creating a temp file
	testFile := filepath.Join(baseDir, ".pacer-doctor-test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return checkResult{
			name:    "Base directory",
			ok:      false,
			message: fmt.Sprintf("%s exists but is not writable", baseDir),
		}
	}
	os.Remove(testFile)

	return checkResult{
		name:    "Base directory",
		ok:      true,
		message: baseDir,
	}
}

func checkGit() checkResult {
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return checkResult{
			name:    "Git",
			ok:      false,
			message: "git command not found",
		}
	}

	return checkResult{
		name:    "Git",
		ok:      true,
		message: string(output[:len(output)-1]), // Remove trailing newline
	}
}

func checkAgent() checkResult {
	cfg, err := config.Load()
	if err != nil {
		return checkResult{
			name:    "Agent",
			ok:      false,
			message: fmt.Sprintf("Could not load config: %v", err),
		}
	}

	if cfg.Agent == "" {
		return checkResult{
			name:    "Agent",
			ok:      true,
			warning: false,
			message: "not configured (worktree management only)",
		}
	}

	// Check if agent is in PATH
	path, err := exec.LookPath(cfg.Agent)
	if err != nil {
		return checkResult{
			name:    "Agent",
			ok:      false,
			message: fmt.Sprintf("%s not found in PATH", cfg.Agent),
		}
	}

	return checkResult{
		name:    "Agent",
		ok:      true,
		message: fmt.Sprintf("%s found at %s", cfg.Agent, path),
	}
}

func checkRepos() []checkResult {
	cfg, err := config.Load()
	if err != nil {
		return []checkResult{{
			name:    "Registered repos",
			ok:      false,
			message: fmt.Sprintf("Could not load config: %v", err),
		}}
	}

	if len(cfg.Repos) == 0 {
		return []checkResult{{
			name:    "Registered repos",
			ok:      true,
			warning: false,
			message: "none registered (use 'pacer repo add' to register)",
		}}
	}

	var results []checkResult
	for name, path := range cfg.Repos {
		expandedPath := config.ExpandPath(path)

		// Check if path exists
		if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
			results = append(results, checkResult{
				name:    fmt.Sprintf("Repo: %s", name),
				ok:      false,
				warning: true,
				message: fmt.Sprintf("path does not exist: %s", expandedPath),
			})
			continue
		}

		// Check if it's a git repo
		if !git.IsGitRepo(expandedPath) {
			results = append(results, checkResult{
				name:    fmt.Sprintf("Repo: %s", name),
				ok:      false,
				warning: true,
				message: fmt.Sprintf("not a git repository: %s", expandedPath),
			})
			continue
		}

		results = append(results, checkResult{
			name:    fmt.Sprintf("Repo: %s", name),
			ok:      true,
			message: expandedPath,
		})
	}

	return results
}

func checkTrustRegistry() checkResult {
	registryPath, err := config.TrustRegistryPath()
	if err != nil {
		return checkResult{
			name:    "Trust registry",
			ok:      false,
			message: fmt.Sprintf("Failed to determine path: %v", err),
		}
	}

	// Check if file exists first
	if _, statErr := os.Stat(registryPath); os.IsNotExist(statErr) {
		return checkResult{
			name:    "Trust registry",
			ok:      true,
			message: "not yet created (no repos trusted)",
			fixFunc: func() error {
				// Create empty trust registry
				registry := &config.TrustRegistry{Repos: make(map[string]config.TrustEntry)}
				return registry.Save()
			},
		}
	}

	registry, err := config.LoadTrustRegistry()
	if err != nil {
		return checkResult{
			name:    "Trust registry",
			ok:      false,
			message: fmt.Sprintf("Failed to load: %v", err),
			fixFunc: func() error {
				// Create fresh trust registry to replace corrupted one
				registry := &config.TrustRegistry{Repos: make(map[string]config.TrustEntry)}
				return registry.Save()
			},
		}
	}

	return checkResult{
		name:    "Trust registry",
		ok:      true,
		message: fmt.Sprintf("%d trusted repo(s)", len(registry.Repos)),
	}
}

// checkStateLocation detects if state.json is at the legacy location
func checkStateLocation() checkResult {
	cfg, err := config.Load()
	if err != nil {
		return checkResult{
			name:    "State location",
			ok:      true, // Don't fail if config can't load
			message: "could not check (config load failed)",
		}
	}

	newPath := config.StatePath()
	legacyPath := config.LegacyStatePath(cfg)

	// If paths are the same (shouldn't happen but check anyway), all good
	if newPath == legacyPath {
		return checkResult{
			name:    "State location",
			ok:      true,
			message: newPath,
		}
	}

	// Check if state exists at LEGACY location
	legacyExists := false
	if _, err := os.Stat(legacyPath); err == nil {
		legacyExists = true
	}

	newExists := false
	if _, err := os.Stat(newPath); err == nil {
		newExists = true
	}

	// Both exist - conflict!
	if legacyExists && newExists {
		return checkResult{
			name:    "State location",
			ok:      false,
			message: fmt.Sprintf("CONFLICT: exists at both %s and %s", legacyPath, newPath),
		}
	}

	// Legacy exists, new doesn't - offer migration
	if legacyExists && !newExists {
		return checkResult{
			name:    "State location",
			ok:      false,
			warning: true,
			message: fmt.Sprintf("at legacy location: %s", legacyPath),
			fixFunc: func() error {
				if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
					return err
				}
				return os.Rename(legacyPath, newPath)
			},
		}
	}

	// Normal case - state at new location (or doesn't exist anywhere)
	return checkResult{
		name:    "State location",
		ok:      true,
		message: newPath,
	}
}

// checkOrphanedWorktrees finds state entries where the directory no longer exists
func checkOrphanedWorktrees() []checkResult {
	cfg, err := config.Load()
	if err != nil {
		return nil // Skip if config can't load
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return nil // Skip if state can't load
	}

	var results []checkResult
	for repoName, worktrees := range state.Worktrees {
		for _, wt := range worktrees {
			wtPath := config.GetWorktreePath(cfg, repoName, wt)
			if _, err := os.Stat(wtPath); os.IsNotExist(err) {
				// Capture variables for closure
				rn, wn := repoName, wt.Name
				results = append(results, checkResult{
					name:    fmt.Sprintf("Worktree: %s/%s", repoName, wt.Name),
					ok:      false,
					warning: true,
					message: fmt.Sprintf("directory missing: %s", wtPath),
					fixFunc: func() error {
						// Reload state to get fresh copy
						freshState, err := config.LoadState(cfg)
						if err != nil {
							return err
						}
						freshState.RemoveWorktree(rn, wn)
						return freshState.Save(cfg)
					},
				})
			}
		}
	}

	return results
}

// checkUntrackedWorktrees finds git worktrees that aren't tracked in pacer state
func checkUntrackedWorktrees() []checkResult {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return nil
	}

	var results []checkResult
	for repoName, repoPath := range cfg.Repos {
		expandedPath := config.ExpandPath(repoPath)

		// Get all git worktrees for this repo
		gitWorktrees, err := git.ListWorktreesDetailed(expandedPath)
		if err != nil {
			continue // Skip repos we can't query
		}

		stateWorktrees := state.Worktrees[repoName]

		for _, gitWt := range gitWorktrees {
			if gitWt.IsMain {
				continue // Skip main repo
			}

			// Check if this git worktree is tracked in pacer state
			found := false
			for _, stateWt := range stateWorktrees {
				stateWtPath := config.GetWorktreePath(cfg, repoName, stateWt)
				if stateWtPath == gitWt.Path {
					found = true
					break
				}
			}

			if !found {
				// Check if this worktree is under pacer's repos directory
				reposDir := cfg.ReposDir()
				// Use strings.HasPrefix on cleaned paths to check containment
				cleanedWtPath := filepath.Clean(gitWt.Path)
				cleanedReposDir := filepath.Clean(reposDir) + string(filepath.Separator)
				if !strings.HasPrefix(cleanedWtPath, cleanedReposDir) {
					continue // Skip worktrees outside pacer's management
				}

				results = append(results, checkResult{
					name:    fmt.Sprintf("Untracked: %s", filepath.Base(gitWt.Path)),
					ok:      false,
					warning: true,
					message: fmt.Sprintf("git worktree not tracked by pacer (branch: %s)", gitWt.Branch),
					// No auto-fix - user should decide to adopt or remove via 'pacer resume' or 'pacer cleanup'
				})
			}
		}
	}

	return results
}

// checkPrunableWorktrees finds repos with stale git worktree entries
func checkPrunableWorktrees() []checkResult {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}

	var results []checkResult
	for repoName, repoPath := range cfg.Repos {
		expandedPath := config.ExpandPath(repoPath)

		// Check if repo is accessible
		if _, err := os.Stat(expandedPath); err != nil {
			continue
		}

		prunable, err := git.HasPrunableWorktrees(expandedPath)
		if err != nil {
			continue // Skip repos we can't check
		}

		if prunable {
			// Capture for closure
			rp := expandedPath
			results = append(results, checkResult{
				name:    fmt.Sprintf("Repo: %s", repoName),
				ok:      false,
				warning: true,
				message: "has stale git worktree entries (prunable)",
				fixFunc: func() error {
					return git.PruneWorktrees(rp)
				},
			})
		}
	}

	return results
}

// checkGlobalHooks verifies global Claude/Cursor hook configuration
func checkGlobalHooks() []checkResult {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return []checkResult{{
			name:    "Global hooks",
			ok:      false,
			message: fmt.Sprintf("could not determine home directory: %v", err),
		}}
	}

	var results []checkResult

	// Check Claude global hook
	claudeSettingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	if hasPacerHook(claudeSettingsPath, "claude") {
		results = append(results, checkResult{
			name:    "Global Claude hook",
			ok:      true,
			message: "pacer inject-context in ~/.claude/settings.json",
		})
	} else {
		cp := claudeSettingsPath // capture for closure
		results = append(results, checkResult{
			name:    "Global Claude hook",
			ok:      false,
			warning: true,
			message: "not found — run 'pacer setup'",
			fixFunc: func() error {
				_, err := mergeClaudeSettingsHooks(cp, false)
				return err
			},
		})
	}

	// Check Cursor global hook
	cursorHooksPath := filepath.Join(homeDir, ".cursor", "hooks.json")
	if hasPacerHook(cursorHooksPath, "cursor") {
		results = append(results, checkResult{
			name:    "Global Cursor hook",
			ok:      true,
			message: "pacer inject-context in ~/.cursor/hooks.json",
		})
	} else {
		cp := cursorHooksPath
		results = append(results, checkResult{
			name:    "Global Cursor hook",
			ok:      false,
			warning: true,
			message: "not found — run 'pacer setup'",
			fixFunc: func() error {
				_, err := mergeCursorHooksJSON(cp, false)
				return err
			},
		})
	}

	// Check Claude /drop command
	claudeDropPath := filepath.Join(homeDir, ".claude", "commands", "drop.md")
	if _, err := os.Stat(claudeDropPath); err == nil {
		results = append(results, checkResult{
			name:    "Global Claude /drop command",
			ok:      true,
			message: "~/.claude/commands/drop.md",
		})
	} else {
		cp := claudeDropPath
		results = append(results, checkResult{
			name:    "Global Claude /drop command",
			ok:      false,
			warning: true,
			message: "not found — run 'pacer setup'",
			fixFunc: func() error {
				_, err := writeDropCommandFile(cp, false)
				return err
			},
		})
	}

	// Check Cursor /drop command
	cursorDropPath := filepath.Join(homeDir, ".cursor", "commands", "drop.md")
	if _, err := os.Stat(cursorDropPath); err == nil {
		results = append(results, checkResult{
			name:    "Global Cursor /drop command",
			ok:      true,
			message: "~/.cursor/commands/drop.md",
		})
	} else {
		cp := cursorDropPath
		results = append(results, checkResult{
			name:    "Global Cursor /drop command",
			ok:      false,
			warning: true,
			message: "not found — run 'pacer setup'",
			fixFunc: func() error {
				_, err := writeDropCommandFile(cp, false)
				return err
			},
		})
	}

	return results
}
