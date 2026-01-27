package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindCopyableFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("SECRET=x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".npmrc"), []byte("registry=..."), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755))

	patterns := []string{".env", ".npmrc", ".env.local", "missing.txt"}
	found := FindCopyableFiles(tmpDir, patterns)

	assert.Equal(t, []string{".env", ".npmrc"}, found)
}

func TestFindCopyableFiles_Deduplicates(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("x"), 0644))

	patterns := []string{".env", ".env", ".env"}
	found := FindCopyableFiles(tmpDir, patterns)

	assert.Equal(t, []string{".env"}, found)
}

func TestFindCopyableFiles_SkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "node_modules"), 0755))

	patterns := []string{"node_modules"}
	found := FindCopyableFiles(tmpDir, patterns)

	assert.Empty(t, found)
}

func TestFindCopyableFiles_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	found := FindCopyableFiles(tmpDir, []string{".env"})
	assert.Empty(t, found)

	found = FindCopyableFiles(tmpDir, nil)
	assert.Empty(t, found)
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

func TestDefaultCopyFiles_ContainsEssentials(t *testing.T) {
	// Verify the defaults include the most common files
	essentials := []string{".env", ".env.local", ".npmrc", ".nvmrc", ".tool-versions"}
	for _, f := range essentials {
		assert.Contains(t, DefaultCopyFiles, f, "DefaultCopyFiles should include %s", f)
	}
}
