package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_CreateAndRead(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)
	sess := &Session{
		SessionID: "test-session-123", Project: "my-project",
		CWD: "/home/user/my-project", Branch: "main",
		Started: time.Now(), LastActive: time.Now(), Status: StatusActive,
	}
	err := reg.Save(sess)
	require.NoError(t, err)
	loaded, err := reg.Get("test-session-123")
	require.NoError(t, err)
	assert.Equal(t, sess.SessionID, loaded.SessionID)
	assert.Equal(t, sess.Project, loaded.Project)
	assert.Equal(t, sess.CWD, loaded.CWD)
	assert.Equal(t, StatusActive, loaded.Status)
}

func TestRegistry_Get_NotFound(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)
	_, err := reg.Get("nonexistent")
	assert.Error(t, err)
}

func TestRegistry_List(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)
	for i, id := range []string{"sess-1", "sess-2", "sess-3"} {
		sess := &Session{
			SessionID: id, Project: "proj", CWD: "/tmp",
			Started: time.Now().Add(-time.Duration(i) * time.Hour),
			LastActive: time.Now().Add(-time.Duration(i) * time.Hour),
			Status: StatusActive,
		}
		require.NoError(t, reg.Save(sess))
	}
	sessions, err := reg.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestRegistry_Update(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)
	sess := &Session{
		SessionID: "update-test", Project: "proj", CWD: "/tmp",
		Started: time.Now(), LastActive: time.Now(), Status: StatusActive,
	}
	require.NoError(t, reg.Save(sess))
	sess.Status = StatusStopped
	sess.Summary = "Did some work"
	require.NoError(t, reg.Save(sess))
	loaded, err := reg.Get("update-test")
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, loaded.Status)
	assert.Equal(t, "Did some work", loaded.Summary)
}

func TestRegistry_Archive(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)
	sess := &Session{
		SessionID: "archive-me", Project: "proj", CWD: "/tmp",
		Started: time.Now().Add(-10 * 24 * time.Hour),
		LastActive: time.Now().Add(-10 * 24 * time.Hour),
		Status: StatusStopped,
	}
	require.NoError(t, reg.Save(sess))
	err := reg.Archive("archive-me")
	require.NoError(t, err)
	sessions, err := reg.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 0)
	archivePath := filepath.Join(dir, "sessions", "archive", "archive-me.json")
	_, err = os.Stat(archivePath)
	assert.NoError(t, err)
}

func TestRegistry_ArchiveStale(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)
	fresh := &Session{
		SessionID: "fresh", Project: "proj", CWD: "/tmp",
		Started: time.Now(), LastActive: time.Now(), Status: StatusActive,
	}
	old := &Session{
		SessionID: "old", Project: "proj", CWD: "/tmp",
		Started: time.Now().Add(-10 * 24 * time.Hour),
		LastActive: time.Now().Add(-10 * 24 * time.Hour),
		Status: StatusStopped,
	}
	require.NoError(t, reg.Save(fresh))
	require.NoError(t, reg.Save(old))
	archived, err := reg.ArchiveStale()
	require.NoError(t, err)
	assert.Equal(t, 1, archived)
	sessions, err := reg.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "fresh", sessions[0].SessionID)
}

func TestRegistry_Delete(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)
	sess := &Session{
		SessionID: "delete-me", Project: "proj", CWD: "/tmp",
		Started: time.Now(), LastActive: time.Now(), Status: StatusStopped,
	}
	require.NoError(t, reg.Save(sess))
	err := reg.Delete("delete-me")
	require.NoError(t, err)
	_, err = reg.Get("delete-me")
	assert.Error(t, err)
}
