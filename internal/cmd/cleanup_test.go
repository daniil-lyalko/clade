package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniil-lyalko/clade/internal/config"
)

func TestArchiveDropbags(t *testing.T) {
	// Create a temporary worktree directory with dropbags
	tmpDir, err := os.MkdirTemp("", "clade-test-wt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dropbags directory structure
	dropbagsDir := filepath.Join(tmpDir, ".clade", "dropbags")
	if err := os.MkdirAll(dropbagsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test dropbag files
	dropbag1 := filepath.Join(dropbagsDir, "DROPBAG-20240115-1430.md")
	dropbag2 := filepath.Join(dropbagsDir, "DROPBAG-20240116-0900.md")
	if err := os.WriteFile(dropbag1, []byte("# Session 1\nTest content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dropbag2, []byte("# Session 2\nMore content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .clade/metadata.json (new path)
	metadataPath := filepath.Join(tmpDir, ".clade", "metadata.json")
	if err := os.WriteFile(metadataPath, []byte(`{"name":"test-wt","label":"spike"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Run archiveDropbags
	archivePath, err := archiveDropbags(tmpDir, "test-repo", "test-wt")
	if err != nil {
		t.Fatalf("archiveDropbags failed: %v", err)
	}

	if archivePath == "" {
		t.Fatal("archiveDropbags returned empty path")
	}

	// Verify archive was created in the right location
	expectedBase := config.ArchiveDir()
	if !strings.HasPrefix(archivePath, expectedBase) {
		t.Errorf("archive path %s doesn't start with expected base %s", archivePath, expectedBase)
	}

	// Verify archive contains the files
	archivedFiles, err := os.ReadDir(archivePath)
	if err != nil {
		t.Fatalf("failed to read archive dir: %v", err)
	}

	foundDropbag1 := false
	foundDropbag2 := false
	foundMetadataJSON := false

	for _, f := range archivedFiles {
		switch f.Name() {
		case "DROPBAG-20240115-1430.md":
			foundDropbag1 = true
		case "DROPBAG-20240116-0900.md":
			foundDropbag2 = true
		case "metadata.json":
			foundMetadataJSON = true
		}
	}

	if !foundDropbag1 {
		t.Error("DROPBAG-20240115-1430.md not found in archive")
	}
	if !foundDropbag2 {
		t.Error("DROPBAG-20240116-0900.md not found in archive")
	}
	if !foundMetadataJSON {
		t.Error("metadata.json not found in archive")
	}

	// Verify content was copied correctly
	content, err := os.ReadFile(filepath.Join(archivePath, "DROPBAG-20240115-1430.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Session 1\nTest content" {
		t.Errorf("unexpected content: %s", string(content))
	}

	// Clean up archive
	os.RemoveAll(archivePath)
	// Try to clean up parent dirs if empty
	os.Remove(filepath.Dir(archivePath))
	os.Remove(config.ArchiveDir())
}

func TestArchiveDropbags_NoDropbags(t *testing.T) {
	// Create a temporary worktree directory WITHOUT dropbags
	tmpDir, err := os.MkdirTemp("", "clade-test-wt-empty")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Run archiveDropbags - should return empty string, no error
	archivePath, err := archiveDropbags(tmpDir, "test-repo", "empty-wt")
	if err != nil {
		t.Fatalf("archiveDropbags failed: %v", err)
	}

	if archivePath != "" {
		t.Errorf("expected empty archive path, got: %s", archivePath)
		os.RemoveAll(archivePath)
	}
}

func TestArchiveDropbags_EmptyDropbagsDir(t *testing.T) {
	// Create a temporary worktree with empty dropbags directory
	tmpDir, err := os.MkdirTemp("", "clade-test-wt-empty-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create empty dropbags directory
	dropbagsDir := filepath.Join(tmpDir, ".clade", "dropbags")
	if err := os.MkdirAll(dropbagsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Run archiveDropbags - should return empty string, no error
	archivePath, err := archiveDropbags(tmpDir, "test-repo", "empty-dir-wt")
	if err != nil {
		t.Fatalf("archiveDropbags failed: %v", err)
	}

	if archivePath != "" {
		t.Errorf("expected empty archive path for empty dropbags dir, got: %s", archivePath)
		os.RemoveAll(archivePath)
	}
}
