package context

import (
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

// ReadDropbag reads the DROPBAG.md file from a directory
func ReadDropbag(dir string) (*DropbagInfo, error) {
	path := filepath.Join(dir, "DROPBAG.md")

	info := &DropbagInfo{
		Exists: false,
	}

	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return info, nil
		}
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	info.Content = strings.TrimSpace(string(data))
	info.ModTime = stat.ModTime()
	info.Exists = true
	info.RelativeAge = formatRelativeTime(stat.ModTime())

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
		return string(rune('0'+mins/10)) + string(rune('0'+mins%10)) + " minutes ago"
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return string(rune('0'+hours/10)) + string(rune('0'+hours%10)) + " hours ago"
	}
	if d < 48*time.Hour {
		return "yesterday"
	}
	days := int(d.Hours() / 24)
	if days < 7 {
		return string(rune('0'+days)) + " days ago"
	}
	return t.Format("Jan 2, 2006")
}
