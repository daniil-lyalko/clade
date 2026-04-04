package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/clade/internal/ui"
)

var migrateToDotCladeFlag bool

func init() {
	migrateCmd.Flags().BoolVar(&migrateToDotCladeFlag, "to-dotclade", false, "Migrate to ~/.clade/ layout (v0.8)")
}

func runMigrateDotClade() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	oldConfigDir := filepath.Join(homeDir, ".config", "clade")
	oldBaseDir := filepath.Join(homeDir, "clade")
	newDir := filepath.Join(homeDir, ".clade")

	ui.Header("Migrating to ~/.clade/")
	fmt.Println()

	if err := doMigrateToDotClade(homeDir, oldConfigDir, oldBaseDir, newDir); err != nil {
		return err
	}

	// Clean up blocking auto-dropbag hooks from settings
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	if removed := removeAutoDropbagHooks(settingsPath); removed > 0 {
		ui.Detail(fmt.Sprintf("Removed %d blocking auto-dropbag hooks from settings", removed))
	}

	ui.Success("Migration complete!")
	ui.Detail("All data now in ~/.clade/")
	ui.Detail("Old paths have symlinks for backward compatibility")
	ui.Detail("Run 'clade setup --force' to update hooks")

	return nil
}

// doMigrateToDotClade performs the actual migration. Extracted for testability.
func doMigrateToDotClade(homeDir, oldConfigDir, oldBaseDir, newDir string) error {
	// 1. Create ~/.clade/ and required subdirectories
	dirs := []string{
		newDir,
		filepath.Join(newDir, "sessions"),
		filepath.Join(newDir, "sessions", "archive"),
		filepath.Join(newDir, "inbox"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	// 2. Move files from ~/.config/clade/ to ~/.clade/
	if oldConfigDir != "" {
		configFiles := []string{"config.json", "state.json", "hooks.yaml", "trusted-repos.json"}
		for _, name := range configFiles {
			src := filepath.Join(oldConfigDir, name)
			dst := filepath.Join(newDir, name)
			migrateSingleFile(src, dst)
		}

		// Move batches/ directory
		migrateSingleDir(filepath.Join(oldConfigDir, "batches"), filepath.Join(newDir, "batches"))
	}

	// 3. Move repos/ from ~/clade/ to ~/.clade/repos/
	if oldBaseDir != "" {
		migrateSingleDir(filepath.Join(oldBaseDir, "repos"), filepath.Join(newDir, "repos"))
		// Also move state.json if it was in old base dir
		migrateSingleFile(filepath.Join(oldBaseDir, "state.json"), filepath.Join(newDir, "state.json"))
	}

	return nil
}

// removeAutoDropbagHooks strips standalone auto-dropbag hooks from Claude settings.
// These were registered by clade v0.7 and block session exit. In v0.8, session-stop
// handles transcript processing via non-blocking triage.
// Returns the number of hooks removed.
func removeAutoDropbagHooks(settingsPath string) int {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return 0
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return 0
	}

	hooksRaw, ok := root["hooks"]
	if !ok {
		return 0
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		return 0
	}

	removed := 0
	for event, hookArrayRaw := range hooks {
		var hookArray []json.RawMessage
		if err := json.Unmarshal(hookArrayRaw, &hookArray); err != nil {
			continue
		}

		var filtered []json.RawMessage
		for _, h := range hookArray {
			if strings.Contains(string(h), "auto-dropbag") {
				removed++
				continue
			}
			filtered = append(filtered, h)
		}

		if len(filtered) != len(hookArray) {
			if len(filtered) == 0 {
				delete(hooks, event)
			} else {
				updated, _ := json.Marshal(filtered)
				hooks[event] = updated
			}
		}
	}

	if removed > 0 {
		hooksJSON, _ := json.Marshal(hooks)
		root["hooks"] = hooksJSON
		output, _ := json.MarshalIndent(root, "", "  ")
		os.WriteFile(settingsPath, output, 0600)
	}

	return removed
}

// migrateSingleFile moves src to dst, then creates a symlink at src pointing to dst.
// Skips if src doesn't exist or dst already exists.
func migrateSingleFile(src, dst string) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return // source doesn't exist
	}

	// Check if src is already a symlink
	if fi, err := os.Lstat(src); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return // already migrated
	}

	if _, err := os.Stat(dst); err == nil {
		// Destination already exists — just create symlink at source
		os.Remove(src)
		os.Symlink(dst, src)
		return
	}

	// Ensure destination directory exists
	os.MkdirAll(filepath.Dir(dst), 0755)

	// Copy file (don't use Rename across potential filesystem boundaries)
	data, err := os.ReadFile(src)
	if err != nil {
		return
	}

	// Preserve original permissions
	info, err := os.Stat(src)
	if err != nil {
		return
	}

	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return
	}

	// Replace source with symlink
	os.Remove(src)
	os.Symlink(dst, src)
}

// migrateSingleDir moves a directory from src to dst.
// Skips if src doesn't exist or dst already exists.
func migrateSingleDir(src, dst string) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	}

	// Check if src is already a symlink
	if fi, err := os.Lstat(src); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return
	}

	if _, err := os.Stat(dst); err == nil {
		// Destination exists — just symlink
		os.RemoveAll(src)
		os.Symlink(dst, src)
		return
	}

	// Move directory
	os.MkdirAll(filepath.Dir(dst), 0755)
	if err := os.Rename(src, dst); err != nil {
		// Rename failed (cross-device) — fall back to leaving in place
		return
	}

	// Create symlink at old location
	os.Symlink(dst, src)
}
