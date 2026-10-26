//go:build windows

package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func lockAgainstRename(t *testing.T, path string) func() {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	unlock := func() { once.Do(func() { _ = windows.CloseHandle(handle) }) }
	t.Cleanup(unlock)
	return unlock
}

func TestWindowsActiveRenameFailureKeepsBoundAndRecovers(t *testing.T) {
	w := testWriter(t, 1024)
	initial := bytes.Repeat([]byte("a"), 1024)
	_, _ = w.Write(initial)
	unlock := lockAgainstRename(t, w.path)
	if _, err := w.Write([]byte("lost")); err == nil {
		t.Fatal("expected sharing violation")
	}
	for i := 0; i < 10; i++ {
		_, _ = w.Write([]byte("lost"))
	}
	data, err := os.ReadFile(w.path)
	if err != nil || !bytes.Equal(data, initial) {
		t.Fatal("failed rotation changed active data")
	}
	unlock()
	w.retryAt = time.Time{}
	if _, err := w.Write([]byte("back")); err != nil {
		t.Fatal(err)
	}
	got := string(retained(t, w.path, 1024))
	if !strings.HasPrefix(got, string(initial)) || !strings.HasSuffix(got, "back") {
		t.Fatal("rename recovery lost evidence")
	}
}

func TestWindowsBackupRenameFailureResumesExactStep(t *testing.T) {
	w := testWriter(t, 1024)
	_, _ = w.Write(bytes.Repeat([]byte("A"), 1024))
	if err := os.WriteFile(w.path+".1", []byte("backup-one"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.path+".2", []byte("backup-two"), 0600); err != nil {
		t.Fatal(err)
	}
	unlock := lockAgainstRename(t, w.path+".1")
	if _, err := w.Write([]byte("lost")); err == nil {
		t.Fatal("expected backup sharing violation")
	}
	// The older backup already moved before the .1 lock stopped rotation.
	if data, _ := os.ReadFile(w.path + ".3"); string(data) != "backup-two" {
		t.Fatal("first rename did not complete")
	}
	_ = w.Close()
	unlock()
	next, err := NewWriter(w.basePath, 1024, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if _, err := next.Write([]byte("back")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(w.path + ".3")
	if err != nil || string(data) != "backup-two" {
		t.Fatalf("oldest survivor lost: %q %v", data, err)
	}
	_ = retained(t, w.path, 1024)
}

func TestWindowsCleanupFailureRetriesOnlyOnScheduledWrite(t *testing.T) {
	now := fixedTime().Add(-12 * time.Hour)
	base := filepath.Join(t.TempDir(), "singbox.log")
	old := DatePath(base, now.AddDate(0, 0, -5))
	if err := os.WriteFile(old, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	unlock := lockAgainstRename(t, old)
	w, err := newWriter(base, 128, 3, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := os.Stat(old); err != nil {
		t.Fatal("locked file changed")
	}
	unlock()
	now = now.Add(59 * time.Minute)
	if _, err := w.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("cleanup retried immediately")
	}
	// Retry on the first write an hour after the failed cleanup, not before.
	now = now.Add(time.Minute)
	if _, err := w.Write([]byte("b")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("cleanup did not retry: %v", err)
	}
}

func TestWindowsPartialWriteReturnsPersistedCount(t *testing.T) {
	w := testWriter(t, 4)
	if err := os.WriteFile(w.path+".1", []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	unlock := lockAgainstRename(t, w.path+".1")
	if n, err := w.Write([]byte("123456")); n != 4 || err == nil {
		t.Fatalf("partial write = %d %v", n, err)
	}
	unlock()
	w.retryAt = time.Time{}
	if n, err := w.Write([]byte("56")); n != 2 || err != nil {
		t.Fatalf("retry = %d %v", n, err)
	}
	if got := string(retained(t, w.path, 4)); got != "old123456" {
		t.Fatalf("retry corrupted bytes: %q", got)
	}
}
