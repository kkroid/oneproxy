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
	_ "time/tzdata"
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
	other, err := NewWriter(w.basePath, 4096, 3)
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
	if got := string(retained(t, w.path, 1024)); got != "back" {
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
	if got := string(retained(t, w.path, 1024)); got != "open-recovered" {
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

func fixedTime() time.Time {
	return time.Date(2026, 3, 1, 23, 59, 59, 0, time.FixedZone("local-test", 8*60*60))
}

func TestDailyRetentionUsesLocalCalendarAndStrictNames(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "oneproxy.log")
	now := fixedTime()
	put := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	expired := []string{"oneproxy-2026-02-24.log", "oneproxy-2026-02-24.log.1", "oneproxy-2026-02-24.log.2", "oneproxy-2026-02-24.log.3"}
	kept := []string{
		"oneproxy-2026-02-25.log", "oneproxy-2026-02-26.log.1", "oneproxy-2026-02-27.log.2", "oneproxy-2026-02-28.log.3",
		"oneproxy-2026-03-02.log", // Future logs are not expired.
		"oneproxy.log", "oneproxy.log.1", "oneproxy.log.oversized",
		"oneproxy-2026-02-24.log.oversized", "oneproxy-2026-02-24.log.4", "oneproxy-2026-02-24.log.01",
		"oneproxy-2026-02-24.log.gz", "oneproxy-2026-02-24.log.bak", "oneproxy-2026-02-30.log", "oneproxy-2026-2-24.log",
		"oneproxy-2025-02-29.log", "oneproxy-0000-01-01.log", "other-2026-02-24.log", "singbox-2026-02-24.log", "xoneproxy-2026-02-24.log",
	}
	for _, name := range append(append([]string{}, expired...), kept...) {
		put(name, []byte("untouched"))
	}
	oversized := "oneproxy-2026-02-23.log"
	large := bytes.Repeat([]byte("x"), 129)
	put(oversized, large)
	if err := os.Mkdir(filepath.Join(dir, "oneproxy-2026-02-22.log"), 0700); err != nil {
		t.Fatal(err)
	}
	w, err := newWriter(base, 128, 3, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, name := range expired {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("expired file remains: %s (%v)", name, err)
		}
	}
	for _, name := range kept {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != "untouched" {
			t.Fatalf("unknown/retained file changed: %s %q %v", name, got, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dir, oversized)); err != nil || !bytes.Equal(got, large) {
		t.Fatal("oversized date file changed")
	}
	if _, err := w.Write([]byte("before midnight")); err != nil {
		t.Fatal(err)
	}
	oldPath := w.path
	// A file added after startup is not scanned on every write.
	put("oneproxy-2026-02-20.log", []byte("late"))
	if _, err := w.Write([]byte("!")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "oneproxy-2026-02-20.log")); err != nil {
		t.Fatal("high-frequency cleanup")
	}
	// UTC is still March 1; the local date has crossed into March 2.
	now = now.Add(2 * time.Second)
	if _, err := w.Write([]byte("after midnight")); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(w.path) != "oneproxy-2026-03-02.log" {
		t.Fatalf("wrong local path: %s", w.path)
	}
	if data, _ := os.ReadFile(oldPath); string(data) != "before midnight!" {
		t.Fatalf("old day changed: %q", data)
	}
	if data, _ := os.ReadFile(w.path); string(data) != "untouchedafter midnight" {
		t.Fatalf("new day not appended: %q", data)
	}
	for _, name := range []string{"oneproxy-2026-02-25.log", "oneproxy-2026-02-20.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("cross-day cleanup missed %s", name)
		}
	}
}

func TestRestartContinuesPartialRotationWithoutDeletingSurvivors(t *testing.T) {
	// Model each durable state left between successful renames and the new open.
	for gap := 1; gap <= 3; gap++ {
		t.Run(fmt.Sprint(gap), func(t *testing.T) {
			now := fixedTime()
			base := filepath.Join(t.TempDir(), "singbox.log")
			path := DatePath(base, now)
			for i, data := range map[int]string{0: "AAAA", 1: "BBBB", 2: "CCCC", 3: "DDDD"} {
				if i == gap {
					continue
				}
				name := path
				if i != 0 {
					name = fmt.Sprintf("%s.%d", path, i)
				}
				if err := os.WriteFile(name, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			w, err := newWriter(base, 4, 3, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			if n, err := w.Write([]byte("Z")); n != 1 || err != nil {
				t.Fatalf("write = %d, %v", n, err)
			}
			_ = w.Close()
			got := string(retained(t, path, 4))
			want := map[int]string{1: "DDDDCCCCAAAAZ", 2: "DDDDBBBBAAAAZ", 3: "CCCCBBBBAAAAZ"}[gap]
			if got != want {
				t.Fatalf("survivors = %q, want %q", got, want)
			}
		})
	}
}

func TestOversizedAndNonRegularDateVolumesStayUntouched(t *testing.T) {
	for _, active := range []bool{true, false} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			now := fixedTime()
			base := filepath.Join(t.TempDir(), "singbox.log")
			path := DatePath(base, now)
			target := path
			if !active {
				target += ".1"
				if err := os.WriteFile(path, []byte("1234"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(target, []byte("12345"), 0600); err != nil {
				t.Fatal(err)
			}
			w, err := newWriter(base, 4, 3, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			if n, err := w.Write([]byte("x")); n != 0 || err == nil {
				t.Fatalf("oversized accepted: %d %v", n, err)
			}
			if got, _ := os.ReadFile(target); string(got) != "12345" {
				t.Fatal("oversized object moved/modified")
			}
			if _, err := os.Stat(path + ".oversized"); !os.IsNotExist(err) {
				t.Fatal("archive created")
			}
		})
	}
	now := fixedTime()
	base := filepath.Join(t.TempDir(), "singbox.log")
	if err := os.Mkdir(DatePath(base, now), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := newWriter(base, 4, 3, func() time.Time { return now }); err == nil {
		t.Fatal("opened directory")
	}
}

func TestLargeWriteWithinRetentionKeepsEveryByte(t *testing.T) {
	w := testWriter(t, 32)
	data := []byte(strings.Repeat("多字节", 10)) // 90 bytes, spanning three files.
	if n, err := w.Write(data); n != len(data) || err != nil {
		t.Fatalf("write = %d %v", n, err)
	}
	if got := retained(t, w.path, 32); !bytes.Equal(got, data) {
		t.Fatalf("large write lost bytes: %q", got)
	}
}

func TestCalendarRetentionAcrossLeapDayYearAndDST(t *testing.T) {
	cases := []struct {
		at            time.Time
		expired, keep string
	}{
		{time.Date(2024, 3, 4, 12, 0, 0, 0, time.UTC), "2024-02-28", "2024-02-29"},
		{time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC), "2025-12-28", "2025-12-29"},
	}
	// Embedded tzdata makes the DST fixture independent of Windows zone data.
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	cases = append(cases, struct {
		at            time.Time
		expired, keep string
	}{
		time.Date(2026, 3, 10, 0, 30, 0, 0, zone), "2026-03-05", "2026-03-06",
	})
	for _, tc := range cases {
		t.Run(tc.at.Format(dateLayout), func(t *testing.T) {
			base := filepath.Join(t.TempDir(), "oneproxy.log")
			for _, date := range []string{tc.expired, tc.keep} {
				name := filepath.Join(filepath.Dir(base), "oneproxy-"+date+".log")
				if err := os.WriteFile(name, []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			w, err := newWriter(base, 32, 3, func() time.Time { return tc.at })
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			for date, exists := range map[string]bool{tc.expired: false, tc.keep: true} {
				_, err := os.Stat(filepath.Join(filepath.Dir(base), "oneproxy-"+date+".log"))
				if (err == nil) != exists {
					t.Fatalf("date %s retained=%v, want %v", date, err == nil, exists)
				}
			}
		})
	}
}
