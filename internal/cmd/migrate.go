package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	migrateForceFlag  bool
	migrateDryRunFlag bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate experiments from v1 to v2 repo-centric structure",
	Long: `Migrate existing experiments from v1 format to v2 repo-centric structure.

v1 structure: ~/clade/experiments/{repo}-{name}/
v2 structure: ~/clade/repos/{repo}/{name}/

This command will:
  1. Backup state.json to state.json.v1.backup
  2. Move experiment folders to the new repo-centric structure
  3. Convert experiments to worktrees in state.json
  4. Update state version to 2

Examples:
  clade migrate              # Show what would be migrated (dry-run)
  clade migrate --dry-run    # Explicit dry-run
  clade migrate --force      # Actually perform the migration`,
	RunE: runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().BoolVarP(&migrateForceFlag, "force", "f", false, "Actually perform the migration")
	migrateCmd.Flags().BoolVarP(&migrateDryRunFlag, "dry-run", "n", false, "Show what would be migrated without making changes")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Check if migration is needed
	if len(state.Experiments) == 0 {
		if state.Version >= 2 {
			ui.Success("Already on v2 state format. No migration needed.")
		} else {
			ui.Info("No v1 experiments to migrate.")
			// Update version anyway
			if !migrateDryRunFlag && !migrateForceFlag {
				ui.Info("Run with --force to update state version to v2.")
			} else if migrateForceFlag {
				state.Version = 2
				if err := state.Save(cfg); err != nil {
					return fmt.Errorf("failed to save state: %w", err)
				}
				ui.Success("State version updated to v2.")
			}
		}
		return nil
	}

	// Default to dry-run unless --force is specified
	isDryRun := !migrateForceFlag || migrateDryRunFlag

	ui.Header("Migration: v1 → v2 (repo-centric)")
	fmt.Println()

	if isDryRun {
		ui.Info("DRY RUN - showing what would be migrated")
		fmt.Println()
	}

	// Plan the migration
	type migrationPlan struct {
		exp      *config.Experiment
		repoName string
		oldPath  string
		newPath  string
		label    string
	}

	var plans []migrationPlan
	var errors []string

	experimentsDir := filepath.Join(cfg.GetBaseDir(), "experiments")

	for key, exp := range state.Experiments {
		repoName := filepath.Base(exp.Repo)

		// Determine label from branch prefix
		label := inferLabelFromBranch(exp.Branch)

		oldPath := exp.Path
		newPath := config.WorktreePath(cfg, repoName, exp.Name)

		// Validate source exists
		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("Experiment '%s' path does not exist: %s", key, oldPath))
			continue
		}

		plans = append(plans, migrationPlan{
			exp:      exp,
			repoName: repoName,
			oldPath:  oldPath,
			newPath:  newPath,
			label:    label,
		})
	}

	// Show migration plan
	ui.Info("Found %d experiment(s) to migrate:", len(plans))
	fmt.Println()

	for _, p := range plans {
		fmt.Printf("  %s %s\n", ui.Cyan(p.exp.Name), ui.Dim("("+p.repoName+")"))
		ui.KeyValue("  Label", p.label)
		ui.KeyValue("  From", p.oldPath)
		ui.KeyValue("  To", p.newPath)
		fmt.Println()
	}

	if len(errors) > 0 {
		ui.Warn("Skipping %d experiment(s) with errors:", len(errors))
		for _, e := range errors {
			ui.Detail("  • %s", e)
		}
		fmt.Println()
	}

	if len(plans) == 0 {
		ui.Info("Nothing to migrate.")
		return nil
	}

	if isDryRun {
		fmt.Println()
		ui.Info("To perform this migration, run:")
		ui.Detail("  clade migrate --force")
		return nil
	}

	// Confirm migration
	if !migrateForceFlag {
		prompt := promptui.Prompt{
			Label:     "Proceed with migration",
			IsConfirm: true,
		}
		_, err := prompt.Run()
		if err != nil {
			ui.Info("Migration cancelled.")
			return nil
		}
	}

	// Create backup
	ui.Info("Creating backup...")
	statePath := config.StatePath(cfg)
	backupPath := statePath + ".v1.backup"

	// Read original state file
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	// Write backup
	if err := os.WriteFile(backupPath, stateData, 0644); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	ui.Success("Backup created: %s", backupPath)

	// Perform migration
	successCount := 0
	failCount := 0

	for _, p := range plans {
		ui.Info("Migrating %s...", p.exp.Name)

		// Ensure target directory parent exists
		targetDir := filepath.Dir(p.newPath)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			ui.Warn("Failed to create directory %s: %v", targetDir, err)
			failCount++
			continue
		}

		// Check if target already exists
		if _, err := os.Stat(p.newPath); err == nil {
			ui.Warn("Target path already exists: %s (skipping)", p.newPath)
			failCount++
			continue
		}

		// Move folder
		if err := os.Rename(p.oldPath, p.newPath); err != nil {
			ui.Warn("Failed to move %s → %s: %v", p.oldPath, p.newPath, err)
			failCount++
			continue
		}

		// Create worktree entry
		wt := &config.Worktree{
			Name:     p.exp.Name,
			Label:    p.label,
			Branch:   p.exp.Branch,
			Ticket:   p.exp.Ticket,
			Created:  p.exp.Created,
			LastUsed: p.exp.LastUsed,
		}
		state.AddWorktree(p.repoName, wt)

		// Remove old experiment entry
		key := config.ExperimentKey(p.exp.Repo, p.exp.Name)
		state.RemoveExperiment(key)

		// Update .clade.json in the moved folder
		cladeJSONPath := filepath.Join(p.newPath, ".clade.json")
		updateCladeJSON(cladeJSONPath, p.label)

		ui.Success("Migrated %s → %s", p.exp.Name, p.newPath)
		successCount++
	}

	// Clean up empty experiments directory
	if entries, err := os.ReadDir(experimentsDir); err == nil && len(entries) == 0 {
		os.Remove(experimentsDir)
		ui.Detail("Removed empty experiments/ directory")
	}

	// Update state version
	state.Version = 2

	// Save updated state
	if err := state.Save(cfg); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	fmt.Println()
	ui.Header("Migration Complete")
	ui.KeyValue("Migrated", fmt.Sprintf("%d worktree(s)", successCount))
	if failCount > 0 {
		ui.KeyValue("Failed", fmt.Sprintf("%d worktree(s)", failCount))
	}
	ui.KeyValue("Backup", backupPath)
	ui.KeyValue("State Version", "2")

	return nil
}

// inferLabelFromBranch determines the label from a branch name
func inferLabelFromBranch(branch string) string {
	// Map branch prefixes to labels
	prefixMap := map[string]string{
		"exp/":     "spike", // v1 experiments become spikes
		"feat/":    "feature",
		"feature/": "feature",
		"fix/":     "bug",
		"bug/":     "bug",
		"spike/":   "spike",
		"chore/":   "chore",
		"hotfix/":  "hotfix",
		"docs/":    "docs",
	}

	for prefix, label := range prefixMap {
		if strings.HasPrefix(branch, prefix) {
			return label
		}
	}

	// Default to spike for experiments (throwaway)
	return "spike"
}

// updateCladeJSON updates the .clade.json file with label information
func updateCladeJSON(path string, label string) {
	// Read existing file
	data, err := os.ReadFile(path)
	if err != nil {
		return // File might not exist, that's OK
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return
	}

	// Update type to "worktree" and add label
	metadata["type"] = "worktree"
	metadata["label"] = label

	// Write back
	updatedData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(path, updatedData, 0644)
}
