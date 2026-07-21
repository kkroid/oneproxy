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
	"unsafe"

	"github.com/kkroid/oneproxy/internal/logger"
	"golang.org/x/sys/windows"
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
	jobObject   windows.Handle
	appLog      *logger.Logger
	lastPID     int // PID of last child, used for scoped cleanup
}

// NewManagerWithLog is like NewManager but accepts a custom log path.
func NewManagerWithLog(singboxPath, configPath, logPath string) *Manager {
	return &Manager{
		configPath:  configPath,
		singboxPath: singboxPath,
		logPath:     logPath,
		stopChan:    make(chan struct{}),
	}
}

// NewManager creates a simple manager with the default log path.
func NewManager(singboxPath, configPath string) *Manager {
	return NewManagerWithLog(singboxPath, configPath, "logs/singbox.log")
}

// SetLogger sets the application logger. Must be called before Start.
func (m *Manager) SetLogger(l *logger.Logger) {
	m.appLog = l
}

// killOrphanedSingBox kills a sing-box.exe process. If pid is non-zero,
// only that specific process is killed (scoped). Otherwise all sing-box.exe
// processes are killed (used when no previous PID is known).
func killOrphanedSingBox(pid int) {
	cmd := func(name string, args ...string) *exec.Cmd {
		c := exec.Command(name, args...)
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		return c
	}
	if pid != 0 {
		_ = cmd("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid)).Run()
	} else {
		_ = cmd("taskkill", "/F", "/IM", "sing-box.exe").Run()
	}
}

// createJobObject creates a Windows Job Object with KILL_ON_JOB_CLOSE.
func createJobObject() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %w", err)
	}

	return job, nil
}

// Start starts the sing-box process.
func (m *Manager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.isRunning {
		return fmt.Errorf("sing-box is already running")
	}

	// Clean up any previous job handle and orphaned process.
	if m.jobObject != 0 {
		windows.CloseHandle(m.jobObject)
		m.jobObject = 0
	}
	killOrphanedSingBox(m.lastPID)

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

	// Create Windows Job Object — if our process dies, OS kills children.
	job, err := createJobObject()
	if err != nil {
		logFile.Close()
		m.logFile = nil
		return fmt.Errorf("failed to create job object: %w", err)
	}
	m.jobObject = job

	// Create command — hide console window
	m.cmd = exec.Command(m.singboxPath, "run", "--disable-color", "-c", m.configPath)
	m.cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	m.cmd.Dir = filepath.Dir(filepath.Dir(m.logPath))
	m.cmd.Env = append(os.Environ(), "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true")
	m.cmd.Stdout = logFile
	m.cmd.Stderr = logFile

	if err := m.cmd.Start(); err != nil {
		logFile.Close()
		m.logFile = nil
		windows.CloseHandle(job)
		m.jobObject = 0
		return fmt.Errorf("failed to start sing-box: %w", err)
	}

	// Assign child process to job object. From this point on, if our process
	// exits for *any* reason, the OS kernel terminates sing-box automatically.
	procHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(m.cmd.Process.Pid))
	if err != nil {
		m.cmd.Process.Kill()
		logFile.Close()
		m.logFile = nil
		windows.CloseHandle(job)
		m.jobObject = 0
		return fmt.Errorf("failed to open process handle: %w", err)
	}
	err = windows.AssignProcessToJobObject(job, procHandle)
	windows.CloseHandle(procHandle)
	if err != nil {
		m.cmd.Process.Kill()
		logFile.Close()
		m.logFile = nil
		windows.CloseHandle(job)
		m.jobObject = 0
		return fmt.Errorf("failed to assign process to job: %w", err)
	}

	m.isRunning = true
	m.lastPID = m.cmd.Process.Pid
	m.stopChan = make(chan struct{})
	go m.monitor(m.cmd, m.stopChan)
	return nil
}

// Stop stops the sing-box process.
func (m *Manager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.isRunning {
		return fmt.Errorf("sing-box is not running")
	}

	// Signal the monitor goroutine that this is an intentional stop.
	close(m.stopChan)

	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
		time.Sleep(200 * time.Millisecond)
	}

	// Release job object handle (child already killed)
	if m.jobObject != 0 {
		windows.CloseHandle(m.jobObject)
		m.jobObject = 0
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

// monitor watches the sing-box process and auto-restarts on unexpected exit.
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
				if m.appLog != nil {
					m.appLog.Error("sing-box restart: cannot open log file: %v", lfErr)
				}
				return
			}
			if m.logFile != nil {
				m.logFile.Close()
			}
			m.logFile = logFile

			cmd = exec.Command(m.singboxPath, "run", "--disable-color", "-c", m.configPath)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			cmd.Dir = filepath.Dir(filepath.Dir(m.logPath))
			cmd.Env = append(os.Environ(), "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true")
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			m.cmd = cmd
			m.lastPID = 0 // will be set on success below
			m.stopChan = make(chan struct{})
			stopChan = m.stopChan
			m.mutex.Unlock()

			if err := cmd.Start(); err != nil {
				if m.appLog != nil {
					m.appLog.Error("sing-box restart attempt %d failed: %v", retry+1, err)
				}
				continue
			}
			m.mutex.Lock()
			m.lastPID = cmd.Process.Pid
			m.mutex.Unlock()
			if m.appLog != nil {
				m.appLog.Warn("sing-box crashed, auto-restarting (attempt %d/%d)", retry+1, maxRetries)
			}
			continue
		}
		// exhausted retries
		m.mutex.Lock()
		m.isRunning = false
		m.mutex.Unlock()
		if err != nil {
			if m.appLog != nil {
				m.appLog.Error("sing-box exited after %d retries: %v", maxRetries, err)
			}
		}
		return
	}
}
