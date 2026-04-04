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
	var migrationErrors []string
	if oldConfigDir != "" {
		configFiles := []string{"config.json", "state.json", "hooks.yaml", "trusted-repos.json"}
		for _, name := range configFiles {
			src := filepath.Join(oldConfigDir, name)
			dst := filepath.Join(newDir, name)
			if err := migrateSingleFile(src, dst); err != nil {
				migrationErrors = append(migrationErrors, fmt.Sprintf("%s: %v", name, err))
			}
		}

		// Move batches/ directory
		if err := migrateSingleDir(filepath.Join(oldConfigDir, "batches"), filepath.Join(newDir, "batches")); err != nil {
			migrationErrors = append(migrationErrors, fmt.Sprintf("batches/: %v", err))
		}
	}

	// 3. Move repos/ from ~/clade/ to ~/.clade/repos/
	if oldBaseDir != "" {
		if err := migrateSingleDir(filepath.Join(oldBaseDir, "repos"), filepath.Join(newDir, "repos")); err != nil {
			migrationErrors = append(migrationErrors, fmt.Sprintf("repos/: %v", err))
		}
		// Also move state.json if it was in old base dir
		if err := migrateSingleFile(filepath.Join(oldBaseDir, "state.json"), filepath.Join(newDir, "state.json")); err != nil {
			migrationErrors = append(migrationErrors, fmt.Sprintf("state.json: %v", err))
		}
	}

	if len(migrationErrors) > 0 {
		return fmt.Errorf("some files failed to migrate:\n  %s", strings.Join(migrationErrors, "\n  "))
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
// Skips (returns nil) if src doesn't exist or is already a symlink.
func migrateSingleFile(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil // source doesn't exist, nothing to do
	}

	// Check if src is already a symlink
	if fi, err := os.Lstat(src); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil // already migrated
	}

	if _, err := os.Stat(dst); err == nil {
		// Destination already exists, create symlink at source
		if err := os.Remove(src); err != nil {
			return fmt.Errorf("failed to remove source %s: %w", src, err)
		}
		if err := os.Symlink(dst, src); err != nil {
			return fmt.Errorf("failed to create symlink %s -> %s: %w", src, dst, err)
		}
		return nil
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination dir for %s: %w", dst, err)
	}

	// Copy file (don't use Rename across potential filesystem boundaries)
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source %s: %w", src, err)
	}

	// Preserve original permissions
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source %s: %w", src, err)
	}

	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to write destination %s: %w", dst, err)
	}

	// Replace source with symlink. Keep a .bak copy instead of deleting
	// (trash > rm per project convention).
	bakPath := src + ".bak"
	if err := os.Rename(src, bakPath); err != nil {
		return fmt.Errorf("failed to back up source %s: %w", src, err)
	}
	if err := os.Symlink(dst, src); err != nil {
		// Restore backup on symlink failure
		os.Rename(bakPath, src)
		return fmt.Errorf("failed to create symlink %s -> %s: %w", src, dst, err)
	}
	return nil
}

// migrateSingleDir moves a directory from src to dst.
// Skips (returns nil) if src doesn't exist or is already a symlink.
func migrateSingleDir(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}

	// Check if src is already a symlink
	if fi, err := os.Lstat(src); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	if _, err := os.Stat(dst); err == nil {
		// Destination exists. Keep a .bak copy instead of deleting
		// (trash > rm per project convention).
		bakPath := src + ".bak"
		if err := os.Rename(src, bakPath); err != nil {
			return fmt.Errorf("failed to back up source dir %s: %w", src, err)
		}
		if err := os.Symlink(dst, src); err != nil {
			// Restore backup on symlink failure
			os.Rename(bakPath, src)
			return fmt.Errorf("failed to create symlink %s -> %s: %w", src, dst, err)
		}
		return nil
	}

	// Move directory
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create parent dir for %s: %w", dst, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("failed to move %s to %s: %w", src, dst, err)
	}

	// Create symlink at old location
	if err := os.Symlink(dst, src); err != nil {
		return fmt.Errorf("failed to create symlink %s -> %s: %w", src, dst, err)
	}
	return nil
}
