//go:build windows

package logger

import (
	"bytes"
	"os"
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
	if !strings.HasPrefix(got, string(initial)) || !strings.Contains(got, "44 bytes lost") || !strings.HasSuffix(got, "back") {
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
	unlock := lockAgainstRename(t, w.path+".2")
	if _, err := w.Write([]byte("lost")); err == nil {
		t.Fatal("expected backup sharing violation")
	}
	if w.rotateStep != 3 {
		t.Fatalf("resume step = %d", w.rotateStep)
	}
	unlock()
	w.retryAt = time.Time{}
	if _, err := w.Write([]byte("back")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(w.path + ".3")
	if err != nil || string(data) != "backup-two" {
		t.Fatalf("oldest survivor lost: %q %v", data, err)
	}
	_ = retained(t, w.path, 1024)
}
