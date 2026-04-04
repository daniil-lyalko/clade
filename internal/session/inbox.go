package session

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type Inbox struct {
	baseDir string
}

func NewInbox(baseDir string) *Inbox {
	return &Inbox{baseDir: baseDir}
}

func (ib *Inbox) inboxDir() string {
	return filepath.Join(ib.baseDir, "inbox")
}

func (ib *Inbox) todayFile() string {
	return filepath.Join(ib.inboxDir(), time.Now().Format("2006-01-02")+".md")
}

func (ib *Inbox) yesterdayFile() string {
	return filepath.Join(ib.inboxDir(), time.Now().AddDate(0, 0, -1).Format("2006-01-02")+".md")
}

// RecentFilePaths returns the paths to today's and yesterday's inbox files.
// The files may or may not exist on disk.
func (ib *Inbox) RecentFilePaths() []string {
	return []string{ib.todayFile(), ib.yesterdayFile()}
}

func FormatInboxEntry(entry *InboxEntry) string {
	timeStr := entry.Time.Format("3:04 PM")
	return fmt.Sprintf("\n### %s | %s | %s\n%s\n", timeStr, entry.Project, entry.EntryType, entry.Message)
}

func (ib *Inbox) Append(entry *InboxEntry) error {
	if err := os.MkdirAll(ib.inboxDir(), 0755); err != nil {
		return fmt.Errorf("failed to create inbox dir: %w", err)
	}
	filePath := ib.todayFile()
	formatted := FormatInboxEntry(entry)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open inbox file: %w", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		today := time.Now().Format("2006-01-02")
		header := fmt.Sprintf("---\ndate: %s\n---\n", today)
		if _, err := f.WriteString(header); err != nil {
			return err
		}
	}
	if _, err := f.WriteString(formatted); err != nil {
		return fmt.Errorf("failed to write inbox entry: %w", err)
	}
	return nil
}

var entryHeaderRe = regexp.MustCompile(`^### (.+?) \| (.+?) \| (.+)$`)

func (ib *Inbox) ReadRecent(offset int64) ([]*InboxEntry, int64, error) {
	var allEntries []*InboxEntry
	if entries, err := ib.readFile(ib.yesterdayFile(), 0); err == nil {
		allEntries = append(allEntries, entries...)
	}
	entries, err := ib.readFile(ib.todayFile(), offset)
	if err != nil && !os.IsNotExist(err) {
		return nil, 0, err
	}
	allEntries = append(allEntries, entries...)
	var newOffset int64
	if info, err := os.Stat(ib.todayFile()); err == nil {
		newOffset = info.Size()
	}
	return allEntries, newOffset, nil
}

func (ib *Inbox) readFile(path string, offset int64) ([]*InboxEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, err
		}
	}

	// Extract the date from the filename (YYYY-MM-DD.md) so that parsed
	// entry times use the correct date rather than today's date.
	fileDate := time.Now()
	base := filepath.Base(path)
	datePart := strings.TrimSuffix(base, ".md")
	if parsed, err := time.ParseInLocation("2006-01-02", datePart, time.Local); err == nil {
		fileDate = parsed
	}

	var entries []*InboxEntry
	var current *InboxEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := entryHeaderRe.FindStringSubmatch(line); matches != nil {
			if current != nil {
				current.Message = strings.TrimSpace(current.Message)
				entries = append(entries, current)
			}
			current = &InboxEntry{
				Project:   strings.TrimSpace(matches[2]),
				EntryType: InboxEntryType(strings.TrimSpace(matches[3])),
				Message:   "",
			}
			if t, err := time.Parse("3:04 PM", strings.TrimSpace(matches[1])); err == nil {
				current.Time = time.Date(fileDate.Year(), fileDate.Month(), fileDate.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
			}
			continue
		}
		if line == "---" || strings.HasPrefix(line, "date:") {
			continue
		}
		if current != nil {
			if current.Message != "" {
				current.Message += "\n"
			}
			current.Message += line
		}
	}
	if current != nil {
		current.Message = strings.TrimSpace(current.Message)
		entries = append(entries, current)
	}
	return entries, scanner.Err()
}

func (ib *Inbox) Cleanup(maxDays int) (int, error) {
	entries, err := os.ReadDir(ib.inboxDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().AddDate(0, 0, -maxDays)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		datePart := strings.TrimSuffix(entry.Name(), ".md")
		fileDate, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			continue
		}
		if fileDate.Before(cutoff) {
			path := filepath.Join(ib.inboxDir(), entry.Name())
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
