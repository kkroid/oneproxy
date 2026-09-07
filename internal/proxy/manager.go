package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kkroid/oneproxy/internal/logger"
)

const (
	defaultStartupWindow = time.Second
	defaultRestartDelay  = 5 * time.Second
)

// Manager manages the sing-box process.
type Manager struct {
	cmd           *exec.Cmd
	configPath    string
	singboxPath   string
	logPath       string
	isRunning     bool
	stopping      bool
	mutex         sync.RWMutex
	stopChan      chan struct{}
	done          chan error
	lifecycleDone chan struct{}
	logFile       *os.File
	process       platformProcess
	appLog        *logger.Logger
	autoRestart   bool
	startupWindow time.Duration
	restartDelay  time.Duration
}

// NewManagerWithLog is like NewManager but accepts a custom log path. Managers
// created by this constructor retain the DLL's automatic restart behavior.
func NewManagerWithLog(singboxPath, configPath, logPath string) *Manager {
	return newManager(singboxPath, configPath, logPath, true, 0)
}

// NewForegroundManagerWithLog creates a foreground manager that never restarts
// sing-box and only reports Start success after the startup observation window.
func NewForegroundManagerWithLog(singboxPath, configPath, logPath string) *Manager {
	return newManager(singboxPath, configPath, logPath, false, defaultStartupWindow)
}

func newManager(singboxPath, configPath, logPath string, autoRestart bool, startupWindow time.Duration) *Manager {
	return &Manager{
		configPath:    configPath,
		singboxPath:   singboxPath,
		logPath:       logPath,
		autoRestart:   autoRestart,
		startupWindow: startupWindow,
		restartDelay:  defaultRestartDelay,
		done:          make(chan error, 1),
	}
}

// NewManager creates a simple manager with the default log path.
func NewManager(singboxPath, configPath string) *Manager {
	return NewManagerWithLog(singboxPath, configPath, "logs/singbox.log")
}

// SetLogger sets the application logger. Must be called before Start.
func (m *Manager) SetLogger(l *logger.Logger) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.appLog = l
}

// Check validates the generated configuration with the pinned sing-box binary.
func (m *Manager) Check() error {
	m.mutex.RLock()
	singboxPath := m.singboxPath
	configPath := m.configPath
	logPath := m.logPath
	m.mutex.RUnlock()

	if err := requireRegularFile(singboxPath, "sing-box binary"); err != nil {
		return err
	}
	if err := requireRegularFile(configPath, "config file"); err != nil {
		return err
	}

	cmd := exec.Command(singboxPath, "check", "--disable-color", "-c", configPath)
	configureProcessCommand(cmd)
	cmd.Dir = filepath.Dir(filepath.Dir(logPath))
	cmd.Env = singboxEnvironment()
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("sing-box check failed: %w", err)
	}
	return fmt.Errorf("sing-box check failed: %w: %s", err, message)
}

// Start starts the sing-box process.
func (m *Manager) Start() error {
	m.mutex.Lock()
	if m.isRunning {
		m.mutex.Unlock()
		return errors.New("sing-box is already running")
	}
	if err := requireRegularFile(m.singboxPath, "sing-box binary"); err != nil {
		m.mutex.Unlock()
		return err
	}
	if err := requireRegularFile(m.configPath, "config file"); err != nil {
		m.mutex.Unlock()
		return err
	}
	if err := prepareLogDir(filepath.Dir(m.logPath)); err != nil {
		m.mutex.Unlock()
		return err
	}

	rotateLogs(filepath.Dir(m.logPath))
	logFile, err := openPrivateLog(m.logPath)
	if err != nil {
		m.mutex.Unlock()
		return err
	}
	m.process.close()
	cmd, err := m.startCommand(logFile)
	if err != nil {
		_ = logFile.Close()
		m.mutex.Unlock()
		return err
	}

	stopChan := make(chan struct{})
	lifecycleDone := make(chan struct{})
	select {
	case <-m.done:
	default:
	}
	m.cmd = cmd
	m.logFile = logFile
	m.isRunning = true
	m.stopping = false
	m.stopChan = stopChan
	m.lifecycleDone = lifecycleDone
	startupWindow := m.startupWindow
	m.mutex.Unlock()

	go m.monitor(cmd, stopChan, m.done, lifecycleDone)
	if startupWindow <= 0 {
		return nil
	}

	timer := time.NewTimer(startupWindow)
	defer timer.Stop()
	select {
	case err := <-m.done:
		if err == nil {
			return errors.New("sing-box exited during startup")
		}
		return err
	case <-lifecycleDone:
		return errors.New("sing-box exited during startup")
	case <-timer.C:
		return nil
	}
}

// Done returns unexpected terminal errors across all process lifecycles.
// Intentional Stop and Restart operations do not publish a result.
func (m *Manager) Done() <-chan error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.done
}

// Stop stops the sing-box process and waits until it has been reaped.
func (m *Manager) Stop() error {
	m.mutex.Lock()
	if !m.isRunning {
		m.mutex.Unlock()
		return nil
	}
	if m.stopping {
		lifecycleDone := m.lifecycleDone
		m.mutex.Unlock()
		<-lifecycleDone
		return nil
	}
	cmd := m.cmd
	stopChan := m.stopChan
	lifecycleDone := m.lifecycleDone
	m.stopping = true
	close(stopChan)
	m.mutex.Unlock()

	if err := m.process.stop(cmd, lifecycleDone); err != nil {
		return fmt.Errorf("failed to stop sing-box: %w", err)
	}
	return nil
}

func (m *Manager) startCommand(logFile *os.File) (*exec.Cmd, error) {
	cmd := exec.Command(m.singboxPath, "run", "--disable-color", "-c", m.configPath)
	configureProcessCommand(cmd)
	cmd.Dir = filepath.Dir(filepath.Dir(m.logPath))
	cmd.Env = singboxEnvironment()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := m.process.start(cmd); err != nil {
		return nil, fmt.Errorf("failed to start sing-box: %w", err)
	}
	return cmd, nil
}

func singboxEnvironment() []string {
	return append(os.Environ(), "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true")
}

// Restart restarts the sing-box process.
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

// IsRunning returns whether sing-box is running.
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.isRunning
}

// GetLogs returns the last N lines from the log file.
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

// SetConfigPath updates the config path.
func (m *Manager) SetConfigPath(path string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.configPath = path
}

// GetPID returns the process ID if running.
func (m *Manager) GetPID() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid
	}
	return 0
}

func requireRegularFile(path, description string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s not found at %s", description, path)
		}
		return fmt.Errorf("inspect %s at %s: %w", description, path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s at %s is not a regular file", description, path)
	}
	return nil
}

func prepareLogDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("failed to secure log directory: %w", err)
	}
	return nil
}

func openPrivateLog(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to secure log file: %w", err)
	}
	return file, nil
}

// rotateLogs deletes log files older than 5 days.
func rotateLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-120 * time.Hour)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// monitor is the only goroutine that calls Cmd.Wait for a process lifecycle.
func (m *Manager) monitor(cmd *exec.Cmd, stopChan <-chan struct{}, done chan<- error, lifecycleDone chan<- struct{}) {
	const maxAttempts = 3
	var terminalErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		waitErr := cmd.Wait()
		if channelClosed(stopChan) {
			terminalErr = nil
			break
		}
		if !m.autoRestart {
			terminalErr = unexpectedExitError(waitErr)
			break
		}
		if attempt == maxAttempts {
			terminalErr = fmt.Errorf("sing-box exited after %d attempts: %w", maxAttempts, unexpectedExitError(waitErr))
			break
		}

		if m.appLog != nil {
			m.appLog.Warn("sing-box crashed, auto-restarting (attempt %d/%d)", attempt+1, maxAttempts)
		}
		timer := time.NewTimer(m.restartDelay)
		select {
		case <-stopChan:
			timer.Stop()
			terminalErr = nil
			attempt = maxAttempts
			continue
		case <-timer.C:
		}

		m.mutex.Lock()
		if channelClosed(stopChan) {
			m.mutex.Unlock()
			terminalErr = nil
			break
		}
		logFile, err := openPrivateLog(m.logPath)
		if err != nil {
			m.mutex.Unlock()
			terminalErr = err
			break
		}
		if m.logFile != nil {
			_ = m.logFile.Close()
		}
		m.logFile = logFile
		cmd, err = m.startCommand(logFile)
		if err != nil {
			_ = logFile.Close()
			m.logFile = nil
			m.mutex.Unlock()
			terminalErr = err
			break
		}
		m.cmd = cmd
		m.mutex.Unlock()
	}

	m.mutex.Lock()
	m.isRunning = false
	m.stopping = false
	m.cmd = nil
	m.process.close()
	if m.logFile != nil {
		_ = m.logFile.Close()
		m.logFile = nil
	}
	m.mutex.Unlock()

	if terminalErr != nil {
		done <- terminalErr
	}
	close(lifecycleDone)
	if terminalErr != nil && m.appLog != nil {
		m.appLog.Error("sing-box stopped: %v", terminalErr)
	}
}

func unexpectedExitError(err error) error {
	if err == nil {
		return errors.New("sing-box exited unexpectedly")
	}
	return fmt.Errorf("sing-box exited unexpectedly: %w", err)
}

func channelClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
