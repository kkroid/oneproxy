package proxy

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Manager manages the sing-box process
type Manager struct {
	cmd          *exec.Cmd
	configPath   string
	singboxPath  string
	logPath      string
	isRunning    bool
	mutex        sync.RWMutex
	stopChan     chan struct{}
	logFile      *os.File
}

// NewManager creates a new proxy manager
func NewManager(singboxPath, configPath string) *Manager {
	return &Manager{
		configPath:  configPath,
		singboxPath: singboxPath,
		logPath:     "logs/singbox.log",
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

	// Open log file
	logFile, err := os.OpenFile(m.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	m.logFile = logFile

	// Create command
	m.cmd = exec.Command(m.singboxPath, "run", "-c", m.configPath)

	// Redirect output to log file
	m.cmd.Stdout = logFile
	m.cmd.Stderr = logFile

	// Start process
	if err := m.cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start sing-box: %w", err)
	}

	m.isRunning = true

	// Monitor process in background
	go m.monitor()

	return nil
}

// Stop stops the sing-box process
func (m *Manager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.isRunning {
		return fmt.Errorf("sing-box is not running")
	}

	// Signal stop
	close(m.stopChan)
	m.stopChan = make(chan struct{})

	// Try graceful shutdown first
	if m.cmd != nil && m.cmd.Process != nil {
		if err := m.cmd.Process.Signal(os.Interrupt); err != nil {
			// If graceful shutdown fails, kill it
			if killErr := m.cmd.Process.Kill(); killErr != nil {
				return fmt.Errorf("failed to kill process: %w", killErr)
			}
		}

		// Wait for process to exit (with timeout)
		done := make(chan error, 1)
		go func() {
			done <- m.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Timeout, force kill
			if m.cmd.Process != nil {
				m.cmd.Process.Kill()
			}
		}
	}

	// Close log file
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

	// Wait a moment before restarting
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

	// Return last N lines
	if len(logLines) > lines {
		return logLines[len(logLines)-lines:], nil
	}

	return logLines, nil
}

// monitor watches the sing-box process
func (m *Manager) monitor() {
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}

	// Wait for process to exit
	err := m.cmd.Wait()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if this was an intentional stop
	select {
	case <-m.stopChan:
		// Intentional stop, do nothing
		return
	default:
		// Unexpected exit
		m.isRunning = false
		if err != nil {
			fmt.Printf("sing-box process exited unexpectedly: %v\n", err)
		}
	}
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
