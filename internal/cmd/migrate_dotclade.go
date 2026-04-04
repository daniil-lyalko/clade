package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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
