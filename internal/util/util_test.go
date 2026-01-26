package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		// Valid names
		{name: "Simple alphanumeric", input: "myfeature", valid: true},
		{name: "With dashes", input: "my-feature", valid: true},
		{name: "With underscores", input: "my_feature", valid: true},
		{name: "Mixed case", input: "MyFeature", valid: true},
		{name: "With numbers", input: "feature123", valid: true},
		{name: "JIRA ticket", input: "PROJ-1234", valid: true},
		{name: "Complex name", input: "PROJ-1234-fix-auth", valid: true},

		// Invalid names
		{name: "Empty string", input: "", valid: false},
		{name: "Starts with dash", input: "-feature", valid: false},
		{name: "Starts with underscore", input: "_feature", valid: false},
		{name: "Contains spaces", input: "my feature", valid: false},
		{name: "Contains special chars", input: "my@feature", valid: false},
		{name: "Contains slashes", input: "my/feature", valid: false},
		{name: "Contains dots", input: "my.feature", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidName(tt.input)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestExtractTicket(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Valid JIRA patterns
		{name: "Standard JIRA", input: "PROJ-1234", expected: "PROJ-1234"},
		{name: "Short project key", input: "AB-1", expected: "AB-1"},
		{name: "Long ticket number", input: "PROJECT-999999", expected: "PROJECT-999999"},
		{name: "With description", input: "PROJ-1234-fix-auth", expected: "PROJ-1234"},
		{name: "Lowercase input", input: "proj-1234", expected: "PROJ-1234"},
		{name: "Mixed case", input: "ProJ-1234", expected: "PROJ-1234"},

		// Invalid patterns
		{name: "No ticket", input: "my-feature", expected: ""},
		{name: "Numbers only", input: "1234", expected: ""},
		{name: "No hyphen", input: "PROJ1234", expected: ""},
		{name: "Ticket in middle", input: "fix-PROJ-1234", expected: ""},
		{name: "Lowercase project", input: "proj-abc", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTicket(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteJSON(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")

	data := map[string]interface{}{
		"name":   "test",
		"value":  123,
		"nested": map[string]string{"key": "value"},
	}

	err := WriteJSON(testFile, data)
	require.NoError(t, err)

	// Verify file exists
	stat, err := os.Stat(testFile)
	require.NoError(t, err)
	assert.Greater(t, stat.Size(), int64(0))

	// Read and verify content
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)

	assert.Contains(t, string(content), "\"name\": \"test\"")
	assert.Contains(t, string(content), "\"value\": 123")
	assert.Contains(t, string(content), "\"nested\"")
}

func TestWriteJSON_InvalidPath(t *testing.T) {
	err := WriteJSON("/nonexistent/directory/file.json", map[string]string{"test": "data"})
	assert.Error(t, err)
}

func TestCopyDir_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create source directory structure
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644))

	// Copy
	err := CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify destination structure
	assert.DirExists(t, dstDir)
	assert.DirExists(t, filepath.Join(dstDir, "subdir"))
	assert.FileExists(t, filepath.Join(dstDir, "file1.txt"))
	assert.FileExists(t, filepath.Join(dstDir, "subdir", "file2.txt"))

	// Verify content
	content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(content1))

	content2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(content2))
}

func TestCopyDir_PreservesPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create source with specific permissions
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "executable.sh"), []byte("#!/bin/bash"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "readonly.txt"), []byte("data"), 0444))

	// Copy
	err := CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify permissions preserved
	execStat, err := os.Stat(filepath.Join(dstDir, "executable.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), execStat.Mode().Perm())

	readonlyStat, err := os.Stat(filepath.Join(dstDir, "readonly.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0444), readonlyStat.Mode().Perm())
}

func TestCopyDir_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create empty source directory
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	// Copy
	err := CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify destination exists
	assert.DirExists(t, dstDir)
}

func TestCopyDir_NonexistentSource(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "nonexistent")
	dstDir := filepath.Join(tmpDir, "dst")

	err := CopyDir(srcDir, dstDir)
	assert.Error(t, err)
}

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create single directory
	singleDir := filepath.Join(tmpDir, "test")
	err := EnsureDir(singleDir)
	require.NoError(t, err)
	assert.DirExists(t, singleDir)

	// Create nested directories
	nestedDir := filepath.Join(tmpDir, "a", "b", "c")
	err = EnsureDir(nestedDir)
	require.NoError(t, err)
	assert.DirExists(t, nestedDir)

	// Call on existing directory (should not error)
	err = EnsureDir(singleDir)
	assert.NoError(t, err)
}
