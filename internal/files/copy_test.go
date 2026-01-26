package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		filename string
		excluded bool
	}{
		{".DS_Store", true},
		{"Thumbs.db", true},
		{"desktop.ini", true},
		{"regular-file.txt", false},
		{".gitignore", false},
		{"somefile.db", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := isExcluded(tt.filename)
			assert.Equal(t, tt.excluded, result)
		})
	}
}

func TestCopyFiles_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create source structure
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "config"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "config", "secrets.json"), []byte("secret"), 0644))

	// Copy specific files
	files := []string{"file1.txt", "config/secrets.json"}
	err := CopyFiles(srcDir, dstDir, files)

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dstDir, "file1.txt"))
	assert.FileExists(t, filepath.Join(dstDir, "config", "secrets.json"))

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(content))
}

func TestCopyFiles_PreservesPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	require.NoError(t, os.MkdirAll(srcDir, 0755))

	// Create file with specific permissions
	execFile := filepath.Join(srcDir, "script.sh")
	require.NoError(t, os.WriteFile(execFile, []byte("#!/bin/bash\n"), 0755))

	// Copy
	err := CopyFiles(srcDir, dstDir, []string{"script.sh"})

	require.NoError(t, err)

	// Verify permissions preserved
	stat, err := os.Stat(filepath.Join(dstDir, "script.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), stat.Mode().Perm())
}

func TestCopyFiles_CreatesNestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create deeply nested file
	nestedDir := filepath.Join(srcDir, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nestedDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "file.txt"), []byte("data"), 0644))

	// Copy
	err := CopyFiles(srcDir, dstDir, []string{"a/b/c/file.txt"})

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dstDir, "a", "b", "c", "file.txt"))
}

func TestCopyFiles_NonexistentSource(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	err := CopyFiles(srcDir, dstDir, []string{"nonexistent.txt"})

	assert.Error(t, err)
}

func TestCopyFile_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.txt")
	dstFile := filepath.Join(tmpDir, "dest.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("test content"), 0644))

	err := copyFile(srcFile, dstFile)

	require.NoError(t, err)
	assert.FileExists(t, dstFile)

	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}

func TestFindGitignored_RequiresGitRepo(t *testing.T) {
	// Test that FindGitignored handles non-git directories gracefully
	tmpDir := t.TempDir()

	files := FindGitignored(tmpDir)

	// Should return nil/empty, not error
	assert.Empty(t, files)
}
