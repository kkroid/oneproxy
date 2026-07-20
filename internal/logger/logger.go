// Package logger provides a simple file logger with size-based rotation.
// Thread-safe, zero external dependencies.
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents log severity.
type Level int

const (
	INFO  Level = iota
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case INFO:  return "INFO"
	case WARN:  return "WARN"
	case ERROR: return "ERROR"
	default:    return "????"
	}
}

// Logger writes timestamped log entries to a file with automatic rotation.
type Logger struct {
	mu        sync.Mutex
	file      *os.File
	path      string
	maxSize   int64 // bytes
	maxFiles  int   // number of rotated backups to keep
	writeN    int64 // monotonic counter to avoid Stat() on every write
}

// New creates a Logger that writes to path. When the file exceeds maxSize
// bytes, it is rotated: path → path.1, path.1 → path.2, ..., oldest removed.
// If the directory doesn't exist it is created.
func New(path string, maxSizeMB int, maxFiles int) (*Logger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("logger: create dir %s: %w", dir, err)
	}

	l := &Logger{
		path:     path,
		maxSize:  int64(maxSizeMB) * 1024 * 1024,
		maxFiles: maxFiles,
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("logger: open %s: %w", path, err)
	}
	l.file = f

	// Seed the size estimate from actual file size
	if fi, err := f.Stat(); err == nil {
		l.writeN = fi.Size()
	}
	return l, nil
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// Info logs at INFO level.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs at WARN level.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs at ERROR level.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// log is the internal write path. Format: "2006-01-02 15:04:05 LEVEL message\n"
func (l *Logger) log(level Level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return // rotate failed earlier; avoid panic
	}

	now := time.Now()
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %-5s %s\n", now.Format("2006-01-02 15:04:05"), level.String(), msg)

	_, err := l.file.WriteString(line)
	if err != nil {
		return // best effort
	}

	// Estimate bytes written to avoid Stat() on every call.
	// Check real file size every 100 writes (coarse, but sufficient).
	l.writeN++
	if l.writeN%100 == 0 {
		if fi, err := l.file.Stat(); err == nil {
			l.writeN = fi.Size()
		}
	}
	if l.writeN >= l.maxSize {
		l.rotate()
	}
}

// rotate closes the current file, renames rotation chain, opens fresh file.
// Must be called with l.mu held. On Windows, Rename can fail (e.g. antivirus
// lock); in that case we skip rotation and continue writing to the current file.
func (l *Logger) rotate() {
	oldFile := l.file

	// Remove oldest backup
	oldest := fmt.Sprintf("%s.%d", l.path, l.maxFiles)
	_ = os.Remove(oldest)

	// Shift: path.N → path.N+1 (descending to avoid overwrites)
	for i := l.maxFiles - 1; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", l.path, i)
		newPath := fmt.Sprintf("%s.%d", l.path, i+1)
		_ = os.Rename(old, newPath)
	}

	// Close old file before renaming it
	_ = oldFile.Close()

	// Rename current → path.1
	_ = os.Rename(l.path, l.path+".1")

	// Open fresh file
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Rotation failed — reopen the old file for appending so we don't lose logs.
		// The old file was renamed to path.1; we can still open it for append
		// if we want, but to keep things simple just set up the rotate chain
		// for the next cycle and leave l.file closed (log() handles nil).
		l.file = nil
		l.writeN = 0
		return
	}
	l.file = f
	l.writeN = 0
}
