package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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
	ready         bool
	stopping      bool
	mutex         sync.RWMutex
	operations    sync.Mutex
	stopChan      chan struct{}
	done          chan error
	lifecycleDone chan struct{}
	logFile       *logger.Writer
	lastOutput    *processOutput
	process       platformProcess
	appLog        *logger.Logger
	autoRestart   bool
	startupWindow time.Duration
	restartDelay  time.Duration
}

// NewManagerWithLog is like NewManager but accepts a custom log path. Managers
// created by this constructor retain the DLL's automatic restart behavior.
func NewManagerWithLog(singboxPath, configPath, logPath string) *Manager {
	return newManager(singboxPath, configPath, logPath, true, defaultStartupWindow)
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
	output := &processOutput{file: io.Discard}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	m.mutex.Lock()
	if !m.isRunning {
		m.lastOutput = output
	}
	m.mutex.Unlock()
	if err == nil {
		return nil
	}
	message := safeCoreDiagnostic(output.tail)
	if message == "" {
		return fmt.Errorf("sing-box check failed: %w", err)
	}
	return fmt.Errorf("sing-box check failed: %w: %s", err, message)
}

// Start starts the sing-box process.
func (m *Manager) Start() error {
	m.operations.Lock()
	defer m.operations.Unlock()
	return m.start()
}

func (m *Manager) start() error {
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

	logFile, err := logger.NewWriter(m.logPath, logger.DefaultMaxSize, logger.DefaultBackups)
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
	m.mutex.Unlock()

	started := make(chan error, 1)
	go m.monitor(cmd, stopChan, m.done, lifecycleDone, started)
	return <-started
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
	m.operations.Lock()
	defer m.operations.Unlock()
	return m.stop()
}

func (m *Manager) stop() error {
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

	if cmd == nil {
		<-lifecycleDone
		return nil
	}
	if err := m.process.stop(cmd, lifecycleDone); err != nil {
		return fmt.Errorf("failed to stop sing-box: %w", err)
	}
	return nil
}

func (m *Manager) startCommand(logFile *logger.Writer) (*exec.Cmd, error) {
	cmd := exec.Command(m.singboxPath, "run", "--disable-color", "-c", m.configPath)
	configureProcessCommand(cmd)
	cmd.Dir = filepath.Dir(filepath.Dir(m.logPath))
	cmd.Env = singboxEnvironment()
	output := &processOutput{file: logFile, appLog: m.appLog}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := m.process.start(cmd); err != nil {
		return nil, fmt.Errorf("failed to start sing-box: %w", err)
	}
	m.lastOutput = output
	return cmd, nil
}

func singboxEnvironment() []string {
	return append(os.Environ(), "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true")
}

// Restart restarts the sing-box process.
func (m *Manager) Restart() error {
	m.operations.Lock()
	defer m.operations.Unlock()
	if !m.IsRunning() {
		return errors.New("sing-box is not running")
	}
	if err := m.stop(); err != nil {
		return fmt.Errorf("failed to stop: %w", err)
	}
	if err := m.start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	return nil
}

// IsRunning returns whether sing-box is running.
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.isRunning && m.ready && !m.stopping
}

// GetLogs returns private, unredacted core output. After Check or a process exit,
// it reads this attempt's final 4KiB in memory; while running it reads the file.
func (m *Manager) GetLogs(lines int) ([]string, error) {
	if lines <= 0 {
		return nil, nil
	}
	m.mutex.RLock()
	output, running := m.lastOutput, m.isRunning
	m.mutex.RUnlock()
	// After an exit, retain this attempt's private diagnostic tail even when
	// persistence failed or the final message crossed a rotation boundary.
	if output != nil && !running {
		output.mu.Lock()
		text := strings.TrimSuffix(string(output.tail), "\n")
		output.mu.Unlock()
		if text == "" {
			return nil, nil
		}
		logLines := strings.Split(text, "\n")
		if len(logLines) > lines {
			logLines = logLines[len(logLines)-lines:]
		}
		return logLines, nil
	}
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
	if m.ready && !m.stopping && m.cmd != nil && m.cmd.Process != nil {
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

// monitor owns each process lifecycle and starts exactly one Wait per child.
func (m *Manager) monitor(cmd *exec.Cmd, stopChan <-chan struct{}, done chan<- error, lifecycleDone chan<- struct{}, started chan<- error) {
	const maxAttempts = 3
	var terminalErr error
	startupReported := false

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Wait drains both output streams before diagnostics are read. An early
		// exit (including an unsuccessful automatic restart) is never retried.
		waited := make(chan error, 1)
		go func(cmd *exec.Cmd) { waited <- cmd.Wait() }(cmd)
		timer := time.NewTimer(m.startupWindow)
		var waitErr error
		earlyExit := false
		select {
		case waitErr = <-waited:
			earlyExit = true
		case <-timer.C:
			select {
			case waitErr = <-waited:
				earlyExit = true
			default:
			}
		}
		timer.Stop()
		if !earlyExit {
			m.mutex.Lock()
			m.ready = true
			m.mutex.Unlock()
			if !startupReported {
				started <- nil
				startupReported = true
			}
			waitErr = <-waited
		}
		m.mutex.Lock()
		m.ready = false
		m.cmd = nil
		m.mutex.Unlock()
		if channelClosed(stopChan) {
			terminalErr = nil
			break
		}
		if earlyExit || !m.autoRestart {
			terminalErr = processExitError(cmd, waitErr)
			break
		}
		if attempt == maxAttempts {
			terminalErr = fmt.Errorf("sing-box exited after %d attempts: %w", maxAttempts, processExitError(cmd, waitErr))
			break
		}

		if m.appLog != nil {
			m.appLog.Warn("sing-box crashed, auto-restarting (attempt %d/%d)", attempt+1, maxAttempts)
		}
		timer = time.NewTimer(m.restartDelay)
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
		m.process.close()
		var err error
		cmd, err = m.startCommand(m.logFile)
		if err != nil {
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
	if !startupReported {
		started <- terminalErr
	} else if terminalErr != nil {
		done <- terminalErr
	}
	close(lifecycleDone)
	m.mutex.Unlock()
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

// processOutput always drains the child, even while disk writes fail. The tail
// belongs only to this attempt. It is never copied verbatim into application logs.
type processOutput struct {
	mu          sync.Mutex
	file        io.Writer
	appLog      *logger.Logger
	tail        []byte
	pending     []byte
	lost        uint64
	lastFailure time.Time
	lastDNS     time.Time
	lastTimeout time.Time
}

func (p *processOutput) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	const limit = 4096
	if len(data) >= limit {
		p.tail = append(p.tail[:0], data[len(data)-limit:]...)
	} else {
		if excess := len(p.tail) + len(data) - limit; excess > 0 {
			p.tail = p.tail[excess:]
		}
		p.tail = append(p.tail, data...)
	}
	n, err := p.file.Write(data)
	if err != nil || n != len(data) {
		p.lost += uint64(len(data) - n)
		if p.appLog != nil && (p.lastFailure.IsZero() || time.Since(p.lastFailure) >= time.Minute) {
			p.appLog.Warn("core log write failed; output is still drained; retrying (%d bytes lost)", p.lost)
			p.lastFailure = time.Now()
		}
	} else if p.lost != 0 {
		if p.appLog != nil {
			p.appLog.Warn("core log writing recovered (%d bytes lost)", p.lost)
		}
		p.lost = 0
		p.lastFailure = time.Time{}
	}
	// Bound memory for arbitrarily long output and handle tokens split across
	// pipe writes. Classifications contain no endpoint, tag or upstream text.
	for _, b := range data {
		p.pending = append(p.pending, b)
		if b == '\n' || len(p.pending) == limit {
			p.reportDiagnostic(p.pending)
			p.pending = p.pending[:0]
		}
	}
	return len(data), nil // Never let os/exec stop draining on a logging error.
}

func coreFailureKind(data []byte) string {
	message := strings.ToLower(string(data))
	// A DNS exchange can itself time out: resolution takes precedence.
	if strings.Contains(message, "lookup ") || strings.Contains(message, "exchange failed") ||
		strings.Contains(message, "dns: ") || strings.Contains(message, "resolve domain") {
		if strings.Contains(message, "failed") || strings.Contains(message, "timeout") ||
			strings.Contains(message, "no such host") || strings.Contains(message, "deadline exceeded") ||
			strings.Contains(message, "name error") || strings.Contains(message, "refused") ||
			strings.Contains(message, "servfail") || strings.Contains(message, "nxdomain") {
			return "dns_resolution_failed"
		}
	}
	if (strings.Contains(message, "connect") || strings.Contains(message, "dial tcp")) &&
		(strings.Contains(message, "timeout") || strings.Contains(message, "timed out") || strings.Contains(message, "deadline exceeded")) {
		return "connection_timeout"
	}
	return ""
}

func (p *processOutput) reportDiagnostic(data []byte) {
	if p.appLog == nil {
		return
	}
	kind := coreFailureKind(data)
	var last *time.Time
	switch kind {
	case "dns_resolution_failed":
		last = &p.lastDNS
	case "connection_timeout":
		last = &p.lastTimeout
	default:
		return
	}
	if last.IsZero() || time.Since(*last) >= time.Minute {
		p.appLog.Warn("core diagnostic: %s (upstream details omitted; repeated events limited to once per minute)", kind)
		*last = time.Now()
	}
}

// Diagnostic text is allowlisted, not regex-redacted: arbitrary upstream errors
// can contain tokens/passwords without recognizable labels. Keep useful startup
// reasons and exit codes without exposing those strings in errors or app logs.
func safeCoreDiagnostic(data []byte) string {
	var messages []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		message := coreFailureKind([]byte(line))
		if message == "" {
			lower := strings.ToLower(line)
			switch {
			case strings.Contains(lower, "address already in use"), strings.Contains(lower, "only one usage of each socket address"):
				message = "listen failed: address already in use"
			case strings.Contains(lower, "invalid generated configuration"), strings.Contains(lower, "decode config"), strings.Contains(lower, "parse config"):
				message = "invalid generated configuration"
			case strings.Contains(lower, "permission denied"), strings.Contains(lower, "access is denied"):
				message = "permission denied"
			case strings.Contains(lower, "no such file"), strings.Contains(lower, "cannot find the file"):
				message = "required file not found"
			case strings.Contains(lower, "certificate"), strings.Contains(lower, "private key"), strings.Contains(lower, "public key"):
				message = "TLS certificate or key error"
			case strings.Contains(lower, "unknown field"), strings.Contains(lower, "unknown outbound"), strings.Contains(lower, "missing"):
				message = "invalid or missing configuration field"
			case strings.Contains(lower, "connection refused"):
				message = "connection refused"
			default:
				message = "unclassified core diagnostic (inspect private core log tail with GetLogs)"
			}
		}
		if !seen[message] {
			messages = append(messages, message)
			seen[message] = true
		}
	}
	return strings.Join(messages, "; ")
}

func processExitError(cmd *exec.Cmd, err error) error {
	exitErr := unexpectedExitError(err)
	if output, ok := cmd.Stderr.(*processOutput); ok {
		output.mu.Lock()
		defer output.mu.Unlock()
		if message := safeCoreDiagnostic(output.tail); message != "" {
			return fmt.Errorf("%w: %s", exitErr, message)
		}
	}
	return exitErr
}
