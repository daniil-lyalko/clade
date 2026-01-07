package files

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CommonIgnoredFiles are files commonly gitignored but needed for running
var CommonIgnoredFiles = []string{
	".env",
	".env.local",
	".envrc",
	".npmrc",
	".yarnrc",
	"config/local.json",
	"config/local.yaml",
	"config/local.yml",
	".vscode/settings.json",
}

// FindGitignored finds files that exist in repoPath but are gitignored
func FindGitignored(repoPath string) []string {
	var found []string

	// Check common patterns
	for _, pattern := range CommonIgnoredFiles {
		fullPath := filepath.Join(repoPath, pattern)
		if _, err := os.Stat(fullPath); err == nil {
			if isGitignored(repoPath, pattern) {
				found = append(found, pattern)
			}
		}
	}

	// Also check for any .env* files
	entries, err := os.ReadDir(repoPath)
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".env") && !entry.IsDir() {
				if isGitignored(repoPath, name) {
					if !contains(found, name) {
						found = append(found, name)
					}
				}
			}
		}
	}

	// Check config/* directory for local configs
	configDir := filepath.Join(repoPath, "config")
	if entries, err := os.ReadDir(configDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			relPath := filepath.Join("config", name)
			// Look for local/secret/dev configs
			if strings.Contains(strings.ToLower(name), "local") ||
				strings.Contains(strings.ToLower(name), "secret") ||
				strings.Contains(strings.ToLower(name), "dev") {
				if isGitignored(repoPath, relPath) {
					if !contains(found, relPath) {
						found = append(found, relPath)
					}
				}
			}
		}
	}

	return found
}

// isGitignored checks if a file is in .gitignore
func isGitignored(repoPath, relPath string) bool {
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Simple pattern matching (exact match or prefix with *)
		if line == relPath {
			return true
		}
		if strings.HasPrefix(line, "*.") && strings.HasSuffix(relPath, line[1:]) {
			return true
		}
		if strings.HasSuffix(line, "/") && strings.HasPrefix(relPath, line) {
			return true
		}
		// Handle patterns like .env*
		if strings.HasSuffix(line, "*") {
			prefix := strings.TrimSuffix(line, "*")
			if strings.HasPrefix(relPath, prefix) {
				return true
			}
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

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
