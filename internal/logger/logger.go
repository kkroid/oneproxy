// Package logger provides thread-safe file logging with daily, size-bounded rotation.
package logger

import (
	"fmt"
	"time"
)

type Level int

const (
	INFO Level = iota
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "????"
	}
}

// Logger retains the application logging API; Writer also accepts core output.
type Logger struct{ writer *Writer }

func New(path string, maxSizeMB int, maxFiles int) (*Logger, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = int(DefaultMaxSize / (1024 * 1024))
	}
	w, err := NewWriter(path, int64(maxSizeMB)*1024*1024, maxFiles)
	if err != nil {
		return nil, err
	}
	return &Logger{writer: w}, nil
}

func (l *Logger) Close() error                             { return l.writer.Close() }
func (l *Logger) Info(format string, args ...interface{})  { l.log(INFO, format, args...) }
func (l *Logger) Warn(format string, args ...interface{})  { l.log(WARN, format, args...) }
func (l *Logger) Error(format string, args ...interface{}) { l.log(ERROR, format, args...) }

func (l *Logger) log(level Level, format string, args ...interface{}) {
	line := fmt.Sprintf("%s %-5s %s\n", time.Now().Format("2006-01-02 15:04:05"), level.String(), fmt.Sprintf(format, args...))
	_, _ = l.writer.Write([]byte(line)) // Best effort; subsequent writes retry persistence.
}
