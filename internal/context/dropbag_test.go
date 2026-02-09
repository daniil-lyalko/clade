package context

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatRelativeTime(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		// Edge cases that previously broke with rune arithmetic
		{
			name:     "2 minutes ago (was broken: showed '02')",
			duration: 2 * time.Minute,
			expected: "2 minutes ago",
		},
		{
			name:     "12 minutes ago (was broken with leading zero)",
			duration: 12 * time.Minute,
			expected: "12 minutes ago",
		},
		{
			name:     "59 minutes ago",
			duration: 59 * time.Minute,
			expected: "59 minutes ago",
		},
		{
			name:     "2 hours ago (was broken: showed '02')",
			duration: 2 * time.Hour,
			expected: "2 hours ago",
		},
		{
			name:     "23 hours ago",
			duration: 23 * time.Hour,
			expected: "23 hours ago",
		},
		{
			name:     "2 days ago (was broken with single digit)",
			duration: 48 * time.Hour,
			expected: "2 days ago",
		},
		{
			name:     "6 days ago",
			duration: 6 * 24 * time.Hour,
			expected: "6 days ago",
		},

		// Normal cases
		{
			name:     "just now",
			duration: 30 * time.Second,
			expected: "just now",
		},
		{
			name:     "1 minute ago (singular)",
			duration: 1 * time.Minute,
			expected: "1 minute ago",
		},
		{
			name:     "1 hour ago (singular)",
			duration: 1 * time.Hour,
			expected: "1 hour ago",
		},
		{
			name:     "yesterday",
			duration: 36 * time.Hour, // Between 24-48 hours
			expected: "yesterday",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := time.Now().Add(-tt.duration)
			result := formatRelativeTime(timestamp)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatRelativeTime_Dates(t *testing.T) {
	// For times > 7 days, should show formatted date
	oldTime := time.Date(2025, 12, 15, 10, 30, 0, 0, time.UTC)
	result := formatRelativeTime(oldTime)

	// Should be in format "Jan 2, 2006"
	assert.Equal(t, "Dec 15, 2025", result)
}

func TestReadDropbag_NoDirectory(t *testing.T) {
	// Test with non-existent directory
	info, err := ReadDropbag("/nonexistent/path")

	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.False(t, info.Exists)
}

func TestReadDropbag_EmptyDirectory(t *testing.T) {
	// Create temp directory with no DROPBAG files
	tmpDir := t.TempDir()

	info, err := ReadDropbag(tmpDir)

	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.False(t, info.Exists)
}

func TestReadDropbag_FindsNewest(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	dropbagsDir := fmt.Sprintf("%s/.clade/dropbags", tmpDir)
	assert.NoError(t, mkdir(dropbagsDir, 0755))

	// Create multiple DROPBAG files with different timestamps
	older := fmt.Sprintf("%s/DROPBAG-2026-01-20-1000.md", dropbagsDir)
	newer := fmt.Sprintf("%s/DROPBAG-2026-01-25-1430.md", dropbagsDir)
	newest := fmt.Sprintf("%s/DROPBAG-2026-01-26-0900.md", dropbagsDir)

	assert.NoError(t, writeFile(older, "Old content", 0644))
	time.Sleep(10 * time.Millisecond)
	assert.NoError(t, writeFile(newer, "Newer content", 0644))
	time.Sleep(10 * time.Millisecond)
	assert.NoError(t, writeFile(newest, "Newest content", 0644))

	// Should find the most recently modified file
	info, err := ReadDropbag(tmpDir)

	assert.NoError(t, err)
	assert.True(t, info.Exists)
	assert.Equal(t, "Newest content", info.Content)
	assert.NotEmpty(t, info.RelativeAge)
}

func TestReadDropbag_SkipsEmptyFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dropbagsDir := fmt.Sprintf("%s/.clade/dropbags", tmpDir)
	assert.NoError(t, mkdir(dropbagsDir, 0755))

	// Create empty DROPBAG file
	emptyFile := fmt.Sprintf("%s/DROPBAG-2026-01-26-1000.md", dropbagsDir)
	assert.NoError(t, writeFile(emptyFile, "", 0644))

	info, err := ReadDropbag(tmpDir)

	assert.NoError(t, err)
	assert.False(t, info.Exists) // Should skip empty files
}

func TestReadDropbag_SkipsNonDropbagFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dropbagsDir := fmt.Sprintf("%s/.clade/dropbags", tmpDir)
	assert.NoError(t, mkdir(dropbagsDir, 0755))

	// Create files that should be ignored
	assert.NoError(t, writeFile(fmt.Sprintf("%s/README.md", dropbagsDir), "Ignored", 0644))
	assert.NoError(t, writeFile(fmt.Sprintf("%s/DROPBAG.txt", dropbagsDir), "Wrong extension", 0644))
	assert.NoError(t, writeFile(fmt.Sprintf("%s/dropbag-lower.md", dropbagsDir), "Wrong case", 0644))

	// Create valid DROPBAG
	validFile := fmt.Sprintf("%s/DROPBAG-2026-01-26-1000.md", dropbagsDir)
	assert.NoError(t, writeFile(validFile, "Valid content", 0644))

	info, err := ReadDropbag(tmpDir)

	assert.NoError(t, err)
	assert.True(t, info.Exists)
	assert.Equal(t, "Valid content", info.Content)
}

// Test helpers
func mkdir(path string, perm uint32) error {
	return os.MkdirAll(path, os.FileMode(perm))
}

func writeFile(path, content string, perm uint32) error {
	return os.WriteFile(path, []byte(content), os.FileMode(perm))
}
