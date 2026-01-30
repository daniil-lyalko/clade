package files

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DefaultCopyFiles are common config files that should be copied to worktrees.
// These are typically gitignored but needed for the project to run.
// Keep this list short and sensible — users can extend it in config.
var DefaultCopyFiles = []string{
	// Environment files
	".env",
	".env.local",
	".env.development",
	".env.development.local",
	".env.test",
	".env.test.local",
	".env.production.local",
	// Package manager configs (may contain registry auth)
	".npmrc",
	".yarnrc",
	".yarnrc.yml",
	// Version manager files
	".tool-versions",
	".nvmrc",
	".python-version",
	".ruby-version",
	".node-version",
	".go-version",
	// IDE / local overrides
	".vscode/settings.json",
}

// FindCopyableFiles checks the source repo for files from the given list that exist.
// Returns only the files that are present and are regular files (not directories or symlinks).
func FindCopyableFiles(repoPath string, patterns []string) []string {
	seen := make(map[string]bool)
	var found []string

	for _, pattern := range patterns {
		if seen[pattern] {
			continue
		}
		seen[pattern] = true

		fullPath := filepath.Join(repoPath, pattern)

		// Use Lstat to detect symlinks (Stat follows symlinks)
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}

		// Skip directories and symlinks
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}

		found = append(found, pattern)
	}

	return found
}


// CopyFiles copies specified files from src to dst directory
func CopyFiles(srcDir, dstDir string, files []string) error {
	for _, relPath := range files {
		srcPath := filepath.Join(srcDir, relPath)
		dstPath := filepath.Join(dstDir, relPath)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	// Security: Check for symlinks before copying
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to copy symlink: %s", src)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get file info from the opened file (not Lstat)
	fileInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return os.Chmod(dst, fileInfo.Mode())
}
