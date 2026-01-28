package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/spf13/cobra"
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
  - Trust registry is readable`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

// checkResult represents a single diagnostic check result
type checkResult struct {
	name    string
	ok      bool
	warning bool // true if it's a warning, false if it's a failure
	message string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	fmt.Println()
	fmt.Println(ui.Bold("Clade Doctor"))
	fmt.Println()

	var results []checkResult
	var warnings, failures int

	// Check config file
	results = append(results, checkConfig())

	// Check state file
	results = append(results, checkState())

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

	// Print results
	for _, r := range results {
		if r.ok {
			fmt.Printf("  %s %s\n", ui.Green("✓"), r.name)
			if r.message != "" {
				fmt.Printf("    %s\n", ui.Dim(r.message))
			}
		} else if r.warning {
			warnings++
			fmt.Printf("  %s %s\n", ui.Yellow("⚠"), r.name)
			if r.message != "" {
				fmt.Printf("    %s\n", ui.Dim(r.message))
			}
		} else {
			failures++
			fmt.Printf("  %s %s\n", ui.Red("✗"), r.name)
			if r.message != "" {
				fmt.Printf("    %s\n", ui.Dim(r.message))
			}
		}
	}

	fmt.Println()

	// Summary
	if failures > 0 {
		ui.Error("%d check(s) failed", failures)
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
		return checkResult{
			name:    "Base directory",
			ok:      true,
			warning: false,
			message: fmt.Sprintf("%s (will be created on first use)", baseDir),
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
	testFile := filepath.Join(baseDir, ".clade-doctor-test")
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
			message: "none registered (use 'clade repo add' to register)",
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

	registry, err := config.LoadTrustRegistry()
	if err != nil {
		return checkResult{
			name:    "Trust registry",
			ok:      false,
			message: fmt.Sprintf("Failed to load: %v", err),
		}
	}

	// Check if file exists
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		return checkResult{
			name:    "Trust registry",
			ok:      true,
			message: "not yet created (no repos trusted)",
		}
	}

	return checkResult{
		name:    "Trust registry",
		ok:      true,
		message: fmt.Sprintf("%d trusted repo(s)", len(registry.Repos)),
	}
}
