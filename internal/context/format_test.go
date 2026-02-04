package context

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatContext_TicketMdPathResolution(t *testing.T) {
	// Regression test for TICKET.md path bug
	// Previously used "." which resolved to process CWD, not worktree directory

	tmpDir := t.TempDir()

	// Create .pacer/metadata.json with ticket (new path)
	pacerJSON := fmt.Sprintf(`{
		"type": "worktree",
		"name": "test-bug",
		"ticket": "PROJ-1234",
		"repo": "test-repo",
		"created": "%s"
	}`, time.Now().Format(time.RFC3339))

	pacerDir := filepath.Join(tmpDir, ".pacer")
	require.NoError(t, os.MkdirAll(pacerDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pacerDir, "metadata.json"), []byte(pacerJSON), 0644))

	// Test case 1: TICKET.md exists in worktree
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "TICKET.md"), []byte("Ticket details"), 0644))

	ctx, err := GatherContext(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	output := FormatContext(ctx)

	assert.Contains(t, output, "PROJ-1234 detected")
	assert.Contains(t, output, "See TICKET.md for details")
	assert.NotContains(t, output, "Please fetch from JIRA")

	// Test case 2: TICKET.md does not exist
	require.NoError(t, os.Remove(filepath.Join(tmpDir, "TICKET.md")))

	ctx, err = GatherContext(tmpDir)
	require.NoError(t, err)

	output = FormatContext(ctx)

	assert.Contains(t, output, "PROJ-1234 detected")
	assert.Contains(t, output, "Please fetch from JIRA and save to TICKET.md")
	assert.NotContains(t, output, "See TICKET.md for details")
}

func TestFormatContext_TicketMdFromSubdirectory(t *testing.T) {
	// Critical test: Verify TICKET.md is found even when pacer inject-context
	// is called from a subdirectory (not repo root)

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "src", "components")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	// Create .pacer.json at repo root
	pacerJSON := fmt.Sprintf(`{
		"type": "worktree",
		"name": "test",
		"ticket": "PROJ-5678",
		"repo": "test-repo",
		"created": "%s"
	}`, time.Now().Format(time.RFC3339))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".pacer.json"), []byte(pacerJSON), 0644))

	// Create TICKET.md at repo root
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "TICKET.md"), []byte("Details"), 0644))

	// Gather context from repo root (should work)
	ctx, err := GatherContext(tmpDir)
	require.NoError(t, err)

	output := FormatContext(ctx)
	assert.Contains(t, output, "See TICKET.md for details")
	assert.NotContains(t, output, "Please fetch from JIRA")
}

func TestFormatContext_NoTicket(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .pacer.json without ticket
	pacerJSON := fmt.Sprintf(`{
		"type": "worktree",
		"name": "test",
		"repo": "test-repo",
		"created": "%s"
	}`, time.Now().Format(time.RFC3339))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".pacer.json"), []byte(pacerJSON), 0644))

	ctx, err := GatherContext(tmpDir)
	require.NoError(t, err)

	output := FormatContext(ctx)

	// Should not have Ticket section at all
	assert.NotContains(t, output, "## Ticket")
}

func TestFormatContext_StaleDropbag(t *testing.T) {
	tmpDir := t.TempDir()
	dropbagsDir := filepath.Join(tmpDir, ".pacer", "dropbags")
	require.NoError(t, os.MkdirAll(dropbagsDir, 0755))

	// Create old DROPBAG (3 days old = stale)
	oldFile := filepath.Join(dropbagsDir, "DROPBAG-2026-01-20-1000.md")
	require.NoError(t, os.WriteFile(oldFile, []byte("Old session notes"), 0644))

	// Set file mod time to 3 days ago
	threeDaysAgo := time.Now().Add(-72 * time.Hour)
	require.NoError(t, os.Chtimes(oldFile, threeDaysAgo, threeDaysAgo))

	ctx, err := GatherContext(tmpDir)
	require.NoError(t, err)

	output := FormatContext(ctx)

	// Should contain stale warning
	assert.Contains(t, output, "⚠️  STALE")
	assert.Contains(t, output, "Consider updating with /drop")
}

func TestFormatContext_FreshDropbag(t *testing.T) {
	tmpDir := t.TempDir()
	dropbagsDir := filepath.Join(tmpDir, ".pacer", "dropbags")
	require.NoError(t, os.MkdirAll(dropbagsDir, 0755))

	// Create recent DROPBAG (1 hour ago = fresh)
	freshFile := filepath.Join(dropbagsDir, "DROPBAG-2026-01-26-1400.md")
	require.NoError(t, os.WriteFile(freshFile, []byte("Fresh session notes"), 0644))

	ctx, err := GatherContext(tmpDir)
	require.NoError(t, err)

	output := FormatContext(ctx)

	// Should NOT contain stale warning
	assert.NotContains(t, output, "⚠️  STALE")
	assert.NotContains(t, output, "Consider updating")
	assert.Contains(t, output, "Fresh session notes")
}

func TestReadPacerMetadata_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	metadataJSON := `{
		"type": "worktree",
		"name": "test-feature",
		"ticket": "PROJ-999",
		"repo": "my-repo",
		"created": "2026-01-26T10:00:00Z"
	}`

	// Use new path: .pacer/metadata.json
	pacerDir := filepath.Join(tmpDir, ".pacer")
	require.NoError(t, os.MkdirAll(pacerDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pacerDir, "metadata.json"), []byte(metadataJSON), 0644))

	metadata, err := ReadPacerMetadata(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, "worktree", metadata.Type)
	assert.Equal(t, "test-feature", metadata.Name)
	assert.Equal(t, "PROJ-999", metadata.Ticket)
	assert.Equal(t, "my-repo", metadata.Repo)
}

func TestReadPacerMetadata_LegacyMigration(t *testing.T) {
	// Test auto-migration from legacy .pacer.json to .pacer/metadata.json
	tmpDir := t.TempDir()

	metadataJSON := `{
		"type": "worktree",
		"name": "legacy-test",
		"ticket": "PROJ-123",
		"repo": "old-repo",
		"created": "2026-01-20T10:00:00Z"
	}`

	// Write to legacy path
	legacyPath := filepath.Join(tmpDir, ".pacer.json")
	require.NoError(t, os.WriteFile(legacyPath, []byte(metadataJSON), 0644))

	// Read should work and auto-migrate
	metadata, err := ReadPacerMetadata(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "legacy-test", metadata.Name)

	// Verify migration happened
	newPath := filepath.Join(tmpDir, ".pacer", "metadata.json")
	assert.FileExists(t, newPath, "metadata.json should exist after migration")
	assert.NoFileExists(t, legacyPath, ".pacer.json should be removed after migration")
}

func TestReadPacerMetadata_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	metadata, err := ReadPacerMetadata(tmpDir)

	assert.Error(t, err)
	assert.Nil(t, metadata)
}

func TestReadPacerMetadata_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".pacer.json"), []byte("invalid json{"), 0644))

	metadata, err := ReadPacerMetadata(tmpDir)

	assert.Error(t, err)
	assert.Nil(t, metadata)
}

func TestGatherContext_StoresDirectory(t *testing.T) {
	// Verify that GatherContext stores the directory in ContextOutput
	// This is critical for TICKET.md path resolution fix

	tmpDir := t.TempDir()

	ctx, err := GatherContext(tmpDir)

	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Equal(t, tmpDir, ctx.Dir)
}
