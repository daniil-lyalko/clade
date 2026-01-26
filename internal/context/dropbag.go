package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DropbagInfo contains information about a DROPBAG.md file
type DropbagInfo struct {
	Content    string
	ModTime    time.Time
	Exists     bool
	RelativeAge string
}

// ReadDropbag reads the most recent DROPBAG-*.md file from .clade/dropbags/
func ReadDropbag(dir string) (*DropbagInfo, error) {
	archiveDir := filepath.Join(dir, ".clade", "dropbags")

	info := &DropbagInfo{
		Exists: false,
	}

	// Read directory entries
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		// Directory doesn't exist or can't read - no DROPBAG available
		return info, nil
	}

	var newestFile string
	var newestTime time.Time

	// Find most recent DROPBAG-*.md file
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "DROPBAG-") || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(archiveDir, entry.Name())
		stat, err := os.Stat(path)
		if err != nil {
			continue
		}

		if newestFile == "" || stat.ModTime().After(newestTime) {
			newestFile = path
			newestTime = stat.ModTime()
		}
	}

	// No DROPBAG files found
	if newestFile == "" {
		return info, nil
	}

	// Read the most recent file
	data, err := os.ReadFile(newestFile)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		// Skip empty files
		return info, nil
	}

	info.Content = content
	info.ModTime = newestTime
	info.Exists = true
	info.RelativeAge = formatRelativeTime(newestTime)

	return info, nil
}

func formatRelativeTime(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		// Fixed: Use fmt.Sprintf instead of broken rune arithmetic
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		// Fixed: Use fmt.Sprintf instead of broken rune arithmetic
		return fmt.Sprintf("%d hours ago", hours)
	}
	if d < 48*time.Hour {
		return "yesterday"
	}
	days := int(d.Hours() / 24)
	if days < 7 {
		// Fixed: Use fmt.Sprintf to handle any number of days correctly
		return fmt.Sprintf("%d days ago", days)
	}
	return t.Format("Jan 2, 2006")
}
