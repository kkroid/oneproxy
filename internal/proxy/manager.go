package proxy

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Manager manages the sing-box process
type Manager struct {
	cmd         *exec.Cmd
	configPath  string
	singboxPath string
	logPath     string
	isRunning   bool
	mutex       sync.RWMutex
	stopChan    chan struct{}
	logFile     *os.File
}

// NewManagerWithLog is like NewManager but accepts a custom log path.
// Use this when the default logs/singbox.log is not writable (e.g. installed dir).
func NewManagerWithLog(singboxPath, configPath, logPath string) *Manager {
	return &Manager{
		configPath:  configPath,
		singboxPath: singboxPath,
		logPath:     logPath,
		stopChan:    make(chan struct{}),
	}
}

// Start starts the sing-box process
func (m *Manager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.isRunning {
		return fmt.Errorf("sing-box is already running")
	}

	// Check if sing-box binary exists
	if _, err := os.Stat(m.singboxPath); os.IsNotExist(err) {
		return fmt.Errorf("sing-box binary not found at %s", m.singboxPath)
	}

	// Check if config exists
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found at %s", m.configPath)
	}

	// Ensure log directory exists
	logDir := filepath.Dir(m.logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Rotate logs older than 5 days
	rotateLogs(logDir)

	// Open log file
	logFile, err := os.OpenFile(m.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	m.logFile = logFile

	// Create command — hide console window
	m.cmd = exec.Command(m.singboxPath, "run", "-c", m.configPath)
	m.cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	// Set cwd to the data directory so sing-box's own log output resolves
	// to a writable path (e.g., ~/.oneproxy/ instead of C:\Program Files\...)
	m.cmd.Dir = filepath.Dir(filepath.Dir(m.logPath))
	m.cmd.Env = append(os.Environ(), "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true")
	m.cmd.Stdout = logFile
	m.cmd.Stderr = logFile

	if err := m.cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start sing-box: %w", err)
	}

	m.isRunning = true
	m.stopChan = make(chan struct{})
	go m.monitor(m.cmd, m.stopChan)
	return nil
}

// Stop stops the sing-box process
func (m *Manager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.isRunning {
		return fmt.Errorf("sing-box is not running")
	}

	close(m.stopChan)

	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}

	if m.logFile != nil {
		m.logFile.Close()
		m.logFile = nil
	}

	m.isRunning = false
	m.cmd = nil
	return nil
}

// Restart restarts the sing-box process
func (m *Manager) Restart() error {
	if err := m.Stop(); err != nil && m.IsRunning() {
		return fmt.Errorf("failed to stop: %w", err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := m.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	return nil
}

// IsRunning returns whether sing-box is running
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.isRunning
}

// GetLogs returns the last N lines from the log file
func (m *Manager) GetLogs(lines int) ([]string, error) {
	file, err := os.Open(m.logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var logLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		logLines = append(logLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}
	if len(logLines) > lines {
		return logLines[len(logLines)-lines:], nil
	}
	return logLines, nil
}

// SetConfigPath updates the config path
func (m *Manager) SetConfigPath(path string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.configPath = path
}

// GetPID returns the process ID if running
func (m *Manager) GetPID() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid
	}
	return 0
}

// rotateLogs deletes log files older than 5 days.
func rotateLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-120 * time.Hour)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".log" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// monitor watches the sing-box process. On unexpected exit, it retries
// up to 3 times with 5-second backoff before giving up.
func (m *Manager) monitor(cmd *exec.Cmd, stopChan chan struct{}) {
	const maxRetries = 3
	for retry := 0; retry < maxRetries; retry++ {
		if cmd == nil || cmd.Process == nil {
			return
		}
		err := cmd.Wait()

		select {
		case <-stopChan:
			return // intentional stop
		default:
		}

		if retry < maxRetries-1 {
			time.Sleep(5 * time.Second)

			m.mutex.Lock()
			if !m.isRunning {
				m.mutex.Unlock()
				return
			}

			logFile, lfErr := os.OpenFile(m.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if lfErr != nil {
				m.isRunning = false
				m.mutex.Unlock()
				fmt.Fprintf(os.Stderr, "sing-box retry %d failed: %v\n", retry+1, lfErr)
				return
			}
			if m.logFile != nil { m.logFile.Close() }
			m.logFile = logFile

			cmd = exec.Command(m.singboxPath, "run", "-c", m.configPath)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			cmd.Env = append(os.Environ(), "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true")
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			m.cmd = cmd
			m.stopChan = make(chan struct{})
			stopChan = m.stopChan
			m.mutex.Unlock()

			if err := cmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "sing-box retry %d failed: %v\n", retry+1, err)
				continue
			}
			fmt.Printf("sing-box auto-restarted (attempt %d/%d)\n", retry+1, maxRetries)
			continue
		}
		// exhausted retries
		m.mutex.Lock()
		m.isRunning = false
		m.mutex.Unlock()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sing-box exited after %d retries: %v\n", maxRetries, err)
		}
		return
	}
}
