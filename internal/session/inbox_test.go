package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInbox_Append(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)
	entry := &InboxEntry{
		Time:      time.Date(2026, 4, 4, 10, 30, 0, 0, time.Local),
		Project:   "leap-complete",
		EntryType: EntryDecision,
		Message:   "API Gateway pointing at old ALB. JWT PRs separate.",
	}
	err := inbox.Append(entry)
	require.NoError(t, err)
	today := time.Now().Format("2006-01-02")
	filePath := filepath.Join(dir, "inbox", today+".md")
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "leap-complete")
	assert.Contains(t, content, "decision")
	assert.Contains(t, content, "API Gateway pointing at old ALB")
}

func TestInbox_AppendMultiple(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)
	entries := []*InboxEntry{
		{Time: time.Now(), Project: "proj-a", EntryType: EntryDecision, Message: "First entry"},
		{Time: time.Now(), Project: "proj-b", EntryType: EntryFYI, Message: "Second entry"},
	}
	for _, e := range entries {
		require.NoError(t, inbox.Append(e))
	}
	today := time.Now().Format("2006-01-02")
	filePath := filepath.Join(dir, "inbox", today+".md")
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "First entry")
	assert.Contains(t, content, "Second entry")
}

func TestInbox_ReadToday(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)
	entry := &InboxEntry{
		Time: time.Now(), Project: "test-proj", EntryType: EntryFYI, Message: "Lambda fix done",
	}
	require.NoError(t, inbox.Append(entry))
	entries, _, err := inbox.ReadRecent(0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 1)
	assert.Equal(t, "Lambda fix done", entries[0].Message)
}

func TestInbox_ReadSinceOffset(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)
	entry1 := &InboxEntry{Time: time.Now(), Project: "proj", EntryType: EntryFYI, Message: "First"}
	require.NoError(t, inbox.Append(entry1))
	today := time.Now().Format("2006-01-02")
	filePath := filepath.Join(dir, "inbox", today+".md")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	offset := info.Size()
	entry2 := &InboxEntry{Time: time.Now(), Project: "proj", EntryType: EntryDecision, Message: "Second"}
	require.NoError(t, inbox.Append(entry2))
	entries, _, err := inbox.ReadRecent(offset)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "Second", entries[0].Message)
}

func TestInbox_Cleanup(t *testing.T) {
	dir := t.TempDir()
	inbox := NewInbox(dir)
	inboxDir := filepath.Join(dir, "inbox")
	require.NoError(t, os.MkdirAll(inboxDir, 0755))
	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	oldPath := filepath.Join(inboxDir, oldDate+".md")
	require.NoError(t, os.WriteFile(oldPath, []byte("old data"), 0644))
	recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	recentPath := filepath.Join(inboxDir, recentDate+".md")
	require.NoError(t, os.WriteFile(recentPath, []byte("recent data"), 0644))
	removed, err := inbox.Cleanup(30)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	_, err = os.Stat(oldPath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(recentPath)
	assert.NoError(t, err)
}

func TestInbox_FormatEntry(t *testing.T) {
	entry := &InboxEntry{
		Time:      time.Date(2026, 4, 4, 10, 30, 0, 0, time.Local),
		Project:   "leap-complete",
		EntryType: EntryDecision,
		Message:   "Going with pg-boss for job queuing.",
	}
	formatted := FormatInboxEntry(entry)
	assert.True(t, strings.HasPrefix(formatted, "\n### "))
	assert.Contains(t, formatted, "leap-complete")
	assert.Contains(t, formatted, "decision")
	assert.Contains(t, formatted, "Going with pg-boss")
}
