package logger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testWriter(t *testing.T, size int64) *Writer {
	t.Helper()
	w, err := NewWriter(filepath.Join(t.TempDir(), "test.log"), size, 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func retained(t *testing.T, path string, max int64) []byte {
	t.Helper()
	var result []byte
	for i := 3; i >= 0; i-- {
		name := path
		if i > 0 {
			name = fmt.Sprintf("%s.%d", path, i)
		}
		data, err := os.ReadFile(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(data)) > max {
			t.Fatalf("%s grew to %d > %d", name, len(data), max)
		}
		result = append(result, data...)
	}
	return result
}

func TestWriterExactBoundaryAndLargeWriteRetention(t *testing.T) {
	w := testWriter(t, 1024)
	first := bytes.Repeat([]byte("a"), 1024)
	if n, err := w.Write(first); n != len(first) || err != nil {
		t.Fatalf("write = %d, %v", n, err)
	}
	if _, err := os.Stat(w.path + ".1"); !os.IsNotExist(err) {
		t.Fatal("rotated before limit needed")
	}
	// The next byte rotates, not the next hundred log calls.
	if _, err := w.Write([]byte("b")); err != nil {
		t.Fatal(err)
	}
	if got := retained(t, w.path, 1024); !bytes.Equal(got, append(first, 'b')) {
		t.Fatal("boundary data changed")
	}
	large := bytes.Repeat([]byte("0123456789"), 700)
	if n, err := w.Write(large); n != len(large) || err != nil {
		t.Fatalf("large write = %d, %v", n, err)
	}
	all := append(append(first, 'b'), large...)
	got := retained(t, w.path, 1024)
	if len(got) > 4096 || !bytes.Equal(got, all[len(all)-len(got):]) {
		t.Fatal("wrong retained suffix or disk bound")
	}
	if _, err := os.Stat(w.path + ".4"); !os.IsNotExist(err) {
		t.Fatal("too many backups")
	}
}

func TestWriterConcurrentRecordsAndReopen(t *testing.T) {
	w := testWriter(t, 4096)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := w.Write([]byte(fmt.Sprintf("%02d:%03d\n", i, j))); err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	other, err := NewWriter(w.path, 4096, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.Write([]byte("reopened\n")); err != nil {
		t.Fatal(err)
	}
	_ = other.Close()
	got := string(retained(t, w.path, 4096))
	for i := 0; i < 8; i++ {
		for j := 0; j < 100; j++ {
			if strings.Count(got, fmt.Sprintf("%02d:%03d\n", i, j)) != 1 {
				t.Fatal("lost/interleaved/duplicated record")
			}
		}
	}
	if !strings.HasSuffix(got, "reopened\n") {
		t.Fatal("reopen lost tail")
	}
	if _, err := w.Write([]byte("closed")); err != os.ErrClosed {
		t.Fatal("closed writer reopened")
	}
}

func TestOversizedPreservedAcrossRestartsAndConflictRecovery(t *testing.T) {
	w := testWriter(t, 1024)
	_ = w.Close()
	old := bytes.Repeat([]byte("legacy-evidence\n"), 10000)
	if err := os.WriteFile(w.path, old, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		next, err := NewWriter(w.path, 1024, 3)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := next.Write(bytes.Repeat([]byte("x"), 1100)); err != nil {
			t.Fatal(err)
		}
		_ = next.Close()
	}
	archive, err := os.ReadFile(w.path + ".oversized")
	if err != nil || !bytes.Equal(archive, old) {
		t.Fatal("old evidence changed")
	}
	_ = retained(t, w.path, 1024)
	// Simulate another oversized active file, while the fixed archive exists.
	if err := os.WriteFile(w.path, old, 0600); err != nil {
		t.Fatal(err)
	}
	next, err := NewWriter(w.path, 1024, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	diagnostic, err := os.Create(filepath.Join(t.TempDir(), "stderr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = diagnostic
	t.Cleanup(func() { os.Stderr = originalStderr; _ = diagnostic.Close() })
	if _, err := next.Write([]byte("lost")); err != errOversizedArchive {
		t.Fatalf("conflict error = %v", err)
	}
	for i := 0; i < 10; i++ {
		_, _ = next.Write([]byte("lost"))
	}
	report, err := os.ReadFile(diagnostic.Name())
	if err != nil || strings.Count(string(report), "log persistence blocked") != 1 ||
		!strings.Contains(string(report), "manually archive the conflicting .oversized") || !strings.Contains(string(report), "4 bytes lost") {
		t.Fatalf("missing/improperly throttled independent diagnostic: %q, %v", report, err)
	}
	active, _ := os.ReadFile(w.path)
	if !bytes.Equal(active, old) {
		t.Fatal("conflict grew or destroyed active file")
	}
	if next.lost != 44 {
		t.Fatalf("lost = %d", next.lost)
	}
	if err := os.Rename(w.path+".oversized", w.path+".manual-archive"); err != nil {
		t.Fatal(err)
	}
	next.retryAt = time.Time{}
	if _, err := next.Write([]byte("recovered")); err != nil {
		t.Fatal(err)
	}
	got := string(retained(t, w.path, 1024))
	if !strings.Contains(got, "44 bytes lost") || !strings.HasSuffix(got, "recovered") {
		t.Fatalf("recovery = %q", got)
	}
	archive, _ = os.ReadFile(w.path + ".oversized")
	if !bytes.Equal(archive, old) {
		t.Fatal("conflicting active evidence not preserved")
	}
}

func TestWriterWriteAndReopenFailuresRecover(t *testing.T) {
	w := testWriter(t, 1024)
	// A real invalid handle makes os.File.Write fail. No fake successful writes.
	_ = w.file.Close()
	if n, err := w.Write([]byte("lost")); n != 0 || err == nil {
		t.Fatalf("write = %d, %v", n, err)
	}
	w.retryAt = time.Time{}
	if _, err := w.Write([]byte("back")); err != nil {
		t.Fatal(err)
	}
	if got := string(retained(t, w.path, 1024)); !strings.Contains(got, "4 bytes lost") || !strings.HasSuffix(got, "back") {
		t.Fatalf("recovery = %q", got)
	}
	// Reopening a path occupied by a directory fails, then recovers on removal.
	_ = w.closeFile()
	if err := os.Rename(w.path, w.path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(w.path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("lost-again")); err == nil {
		t.Fatal("open unexpectedly succeeded")
	}
	if err := os.Remove(w.path); err != nil {
		t.Fatal(err)
	}
	w.retryAt = time.Time{}
	if _, err := w.Write([]byte("open-recovered")); err != nil {
		t.Fatal(err)
	}
	if got := string(retained(t, w.path, 1024)); !strings.Contains(got, "10 bytes lost") {
		t.Fatalf("reopen recovery = %q", got)
	}
}

func TestLoggerUsesBytesAndDefaultPolicy(t *testing.T) {
	l, err := New(filepath.Join(t.TempDir(), "app.log"), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if l.writer.maxSize != DefaultMaxSize || l.writer.maxFiles != 3 {
		t.Fatal("wrong defaults")
	}
	l.writer.maxSize = 256
	l.Info("%s", strings.Repeat("中", 100))
	if _, err := os.Stat(l.writer.path + ".1"); err != nil {
		t.Fatal("one multi-byte record did not rotate")
	}
	l.Warn("warning")
	l.Error("error")
	got := string(retained(t, l.writer.path, 256))
	if !strings.Contains(got, "INFO") || !strings.Contains(got, "WARN") || !strings.Contains(got, "ERROR") {
		t.Fatalf("API lost severity: %q", got)
	}
}
