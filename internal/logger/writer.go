package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultMaxSize int64 = 64 * 1024 * 1024
const DefaultBackups = 3
const dateLayout = "2006-01-02"

// Writer owns the only append handle for a log. One owner per base path is
// required. Writes are split at the byte limit and can span daily volumes.
type Writer struct {
	mu          sync.Mutex
	basePath    string
	path        string
	maxSize     int64
	maxFiles    int
	file        *os.File
	size        int64
	closed      bool
	now         func() time.Time
	nextCleanup time.Time
	retryAt     time.Time
	lastError   error
}

// DatePath maps a logical path (e.g. singbox.log) to the date in at's location.
// Production callers use time.Now(), which uses the system's local time zone.
func DatePath(base string, at time.Time) string {
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext) + "-" + at.Format(dateLayout) + ext
}

func NewWriter(path string, maxSize int64, maxFiles int) (*Writer, error) {
	return newWriter(path, maxSize, maxFiles, time.Now)
}

func newWriter(path string, maxSize int64, maxFiles int, now func() time.Time) (*Writer, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if maxFiles <= 0 {
		maxFiles = DefaultBackups
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	at := now()
	w := &Writer{basePath: path, path: DatePath(path, at), maxSize: maxSize, maxFiles: maxFiles, now: now}
	w.cleanup(at)
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

// Path returns the last opened date path; before the first post-midnight write
// this is still the previous day's file.
func (w *Writer) Path() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path
}

func (w *Writer) open() error {
	// Refuse symlinks and other unowned objects instead of following them.
	if info, err := os.Lstat(w.path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("logger: not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	// An externally oversized date file is left untouched; Write will fail.
	if info.Size() <= w.maxSize {
		if err := f.Chmod(0600); err != nil {
			_ = f.Close()
			return err
		}
	}
	w.file, w.size = f, info.Size()
	return nil
}

func (w *Writer) closeFile() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return w.closeFile()
}

func (w *Writer) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if len(data) == 0 {
		return 0, nil
	}
	at := w.now()
	path := DatePath(w.basePath, at)
	if path != w.path {
		if err := w.closeFile(); err != nil {
			return 0, err
		}
		w.path = path
		w.retryAt = time.Time{}
		w.cleanup(at)
	} else if !at.Before(w.nextCleanup) {
		w.cleanup(at)
	}
	if at.Before(w.retryAt) {
		return 0, w.lastError
	}
	n, err := w.write(data)
	if err != nil {
		w.retryAt, w.lastError = at.Add(time.Second), err
	} else {
		w.retryAt, w.lastError = time.Time{}, nil
	}
	return n, err
}

// write is called under mu. Errors return the exact persisted byte count;
// process output callers continue draining even when persistence is unavailable.
func (w *Writer) write(data []byte) (int, error) {
	total := 0
	for len(data) != 0 {
		if w.file == nil {
			if err := w.open(); err != nil {
				return total, err
			}
		}
		if w.size > w.maxSize {
			_ = w.closeFile()
			return total, errors.New("logger: date file exceeds size limit")
		}
		if w.size == w.maxSize {
			if err := w.closeFile(); err != nil {
				return total, err
			}
			if err := w.rotate(); err != nil {
				return total, err
			}
			continue
		}
		length := int64(len(data))
		if available := w.maxSize - w.size; length > available {
			length = available
		}
		n, err := w.file.Write(data[:int(length)])
		total += n
		w.size += int64(n)
		data = data[n:]
		if err == nil && int64(n) != length {
			err = io.ErrShortWrite
		}
		if err != nil {
			_ = w.closeFile()
			return total, err
		}
	}
	return total, nil
}

func (w *Writer) volume(n int) string { return fmt.Sprintf("%s.%d", w.path, n) }

func (w *Writer) rotate() error {
	// Fill the first vacant slot. A failed rename (or restart between renames)
	// leaves that vacancy on disk, so retries do not shift survivors twice.
	gap := 0
	for i := 1; i <= w.maxFiles; i++ {
		info, err := os.Lstat(w.volume(i))
		if os.IsNotExist(err) {
			if gap == 0 {
				gap = i
			}
		} else if err != nil {
			return err
		} else if !info.Mode().IsRegular() || info.Size() > w.maxSize {
			return errors.New("logger: volume is not a bounded regular file")
		}
	}
	if gap == 0 {
		if err := os.Remove(w.volume(w.maxFiles)); err != nil {
			return err
		}
		gap = w.maxFiles
	}
	for i := gap; i > 1; i-- {
		if err := os.Rename(w.volume(i-1), w.volume(i)); err != nil {
			return err
		}
	}
	return os.Rename(w.path, w.volume(1))
}

// ownedDate accepts only this stream's exact date name and configured volumes.
func (w *Writer) ownedDate(name string) (string, bool) {
	base := filepath.Base(w.basePath)
	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext) + "-"
	if !strings.HasPrefix(name, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(name, prefix)
	if len(rest) < len(dateLayout) {
		return "", false
	}
	date, suffix := rest[:len(dateLayout)], rest[len(dateLayout):]
	if parsed, err := time.Parse(dateLayout, date); err != nil || parsed.Year() < 1 {
		return "", false
	}
	if suffix == ext {
		return date, true
	}
	for i := 1; i <= w.maxFiles; i++ {
		if suffix == ext+"."+strconv.Itoa(i) {
			return date, true
		}
	}
	return "", false
}

func (w *Writer) cleanup(at time.Time) {
	// Once per local day; locked files/read failures retry at most hourly on writes.
	w.nextCleanup = time.Date(at.Year(), at.Month(), at.Day()+1, 0, 0, 0, 0, at.Location())
	retry := func() {
		if next := at.Add(time.Hour); next.Before(w.nextCleanup) {
			w.nextCleanup = next
		}
	}
	entries, err := os.ReadDir(filepath.Dir(w.basePath))
	if err != nil {
		retry()
		return
	}
	cutoff := at.AddDate(0, 0, -4).Format(dateLayout)
	for _, entry := range entries {
		date, owned := w.ownedDate(entry.Name())
		if !owned || date >= cutoff || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			retry()
			continue
		}
		if !info.Mode().IsRegular() || info.Size() > w.maxSize {
			continue
		}
		if err := os.Remove(filepath.Join(filepath.Dir(w.basePath), entry.Name())); err != nil && !os.IsNotExist(err) {
			retry()
		}
	}
}
