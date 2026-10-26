package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const DefaultMaxSize int64 = 10 * 1024 * 1024
const DefaultBackups = 3

var errOversizedArchive = errors.New("logger: oversized archive already exists; manual archival required")

// Writer owns the only append handle for a log. One owner per path is required.
// Each file is bounded even when a single Write exceeds the limit; records can
// span files. An oversized pre-upgrade active file is preserved as .oversized.
type Writer struct {
	mu         sync.Mutex
	path       string
	maxSize    int64
	maxFiles   int
	file       *os.File
	size       int64
	closed     bool
	rotateStep int // resumable: remove oldest, shift backups, rename active
	lost       uint64
	lastReport time.Time
	retryAt    time.Time
	lastError  error
}

func NewWriter(path string, maxSize int64, maxFiles int) (*Writer, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if maxFiles <= 0 {
		maxFiles = DefaultBackups
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	w := &Writer{path: path, maxSize: maxSize, maxFiles: maxFiles}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return errors.New("logger: not a regular file")
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
	if time.Now().Before(w.retryAt) {
		return w.failed(0, len(data), w.lastError)
	}
	// Recovery is visible in the file even for GUI applications without stderr.
	if w.lost != 0 {
		message := []byte(fmt.Sprintf("\n[oneproxy log recovered: %d bytes lost during logging failure]\n", w.lost))
		if _, err := w.write(message); err != nil {
			return w.failed(0, len(data), err)
		}
		w.lost = 0
		w.lastReport = time.Time{}
	}
	n, err := w.write(data)
	if err != nil {
		return w.failed(n, len(data), err)
	}
	w.retryAt = time.Time{}
	w.lastError = nil
	return n, nil
}

func (w *Writer) failed(n, requested int, err error) (int, error) {
	w.lost += uint64(requested - n)
	if !time.Now().Before(w.retryAt) {
		w.retryAt = time.Now().Add(time.Second)
	}
	w.lastError = err
	// Never include paths, data or upstream error strings in fallback diagnostics.
	if w.lastReport.IsZero() || time.Since(w.lastReport) >= time.Minute {
		reason := "log persistence blocked; retrying at most once per second"
		if errors.Is(err, errOversizedArchive) {
			reason = "log persistence blocked; manually archive the conflicting .oversized file before retry"
		}
		_, _ = fmt.Fprintf(os.Stderr, "OneProxy %s (%d bytes lost)\n", reason, w.lost)
		w.lastReport = time.Now()
	}
	return n, err
}

// write is called under mu, including for recovery records.
func (w *Writer) write(data []byte) (int, error) {
	total := 0
	for len(data) != 0 {
		if w.rotateStep != 0 {
			if err := w.rotate(); err != nil {
				return total, err
			}
		}
		if w.file == nil {
			if err := w.open(); err != nil {
				return total, err
			}
		}
		if w.size > w.maxSize {
			if err := w.preserveOversized(); err != nil {
				return total, err
			}
			continue
		}
		if w.size == w.maxSize {
			if err := w.closeFile(); err != nil {
				return total, err
			}
			w.rotateStep = w.maxFiles + 1
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
			_ = w.closeFile() // Reopen and stat on the next call; no permanent nil sink.
			return total, err
		}
	}
	return total, nil
}

func (w *Writer) preserveOversized() error {
	if err := w.closeFile(); err != nil {
		return err
	}
	archive := w.path + ".oversized"
	if _, err := os.Lstat(archive); err == nil {
		return errOversizedArchive
	} else if !os.IsNotExist(err) {
		return err
	}
	// Rename only: no multi-gigabyte read/copy, truncation, or automatic deletion.
	return os.Rename(w.path, archive)
}

func (w *Writer) rotate() error {
	for w.rotateStep > 0 {
		step := w.rotateStep
		var err error
		switch {
		case step == w.maxFiles+1:
			err = os.Remove(fmt.Sprintf("%s.%d", w.path, w.maxFiles))
		case step > 1:
			err = os.Rename(fmt.Sprintf("%s.%d", w.path, step-1), fmt.Sprintf("%s.%d", w.path, step))
		default:
			err = os.Rename(w.path, w.path+".1")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		w.rotateStep-- // Resume this exact step after a Windows sharing violation.
	}
	return nil
}
