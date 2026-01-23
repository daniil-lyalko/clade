package files

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExcludeFiles - OS junk files, never prompt for these
var ExcludeFiles = []string{
	".DS_Store",
	"Thumbs.db",
	"desktop.ini",
	".Spotlight-V100",
	".Trashes",
	"ehthumbs.db",
}

// FindGitignored finds all gitignored FILES (not directories) that exist in the repo.
// Uses git's native gitignore parsing which handles:
// - .gitignore at any level
// - Global gitignore (~/.config/git/ignore)
// - .git/info/exclude
func FindGitignored(repoPath string) []string {
	// Use git to find all ignored files that exist
	// --others: untracked files only
	// --ignored: show ignored files
	// --exclude-standard: use standard ignore rules (.gitignore, global, etc.)
	cmd := exec.Command("git", "ls-files", "--others", "--ignored", "--exclude-standard")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// Fallback to empty if git command fails
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var found []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip OS junk files
		if isExcluded(filepath.Base(line)) {
			continue
		}

		// Verify it's a file (not a directory) and exists
		fullPath := filepath.Join(repoPath, line)
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}

		found = append(found, line)
	}

	return found
}

// isExcluded checks if a filename is in the OS junk exclusion list
func isExcluded(filename string) bool {
	for _, excluded := range ExcludeFiles {
		if filename == excluded {
			return true
		}
	}
	return false
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
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
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

	return os.Chmod(dst, srcInfo.Mode())
}
