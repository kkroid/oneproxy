package proxy

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/kkroid/oneproxy/internal/logger"
)

// DNSFlusher handles DNS cache flushing
type DNSFlusher struct {
	lastFlush    time.Time
	flushLock    sync.Mutex
	flushCooldown time.Duration
	appLog        *logger.Logger
}

func NewDNSFlusher() *DNSFlusher {
	return &DNSFlusher{
		flushCooldown: 10 * time.Second,
	}
}

// SetCooldown sets the minimum interval between flushes.
func (f *DNSFlusher) SetCooldown(d time.Duration) { f.flushCooldown = d }

// SetLogger sets the logger for recording errors.
func (f *DNSFlusher) SetLogger(l *logger.Logger) { f.appLog = l }

func (f *DNSFlusher) FlushAll(manager *Manager) error {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()

	if elapsed := time.Since(f.lastFlush); elapsed < f.flushCooldown {
		if f.appLog != nil {
			f.appLog.Warn("DNS flush skipped — cooldown (%v remain)", (f.flushCooldown - elapsed).Round(time.Second))
		}
		return fmt.Errorf("DNS flush called too frequently, please wait")
	}

	if err := f.FlushSystemDNS(); err != nil {
		if f.appLog != nil {
			f.appLog.Error("DNS flush failed: %v", err)
		}
		return fmt.Errorf("failed to flush system DNS: %w", err)
	}

	if manager != nil && manager.IsRunning() {
		if f.appLog != nil {
			f.appLog.Info("DNS flushed — restarting sing-box")
		}
		if err := manager.Restart(); err != nil {
			if f.appLog != nil {
				f.appLog.Error("sing-box restart failed: %v", err)
			}
			return fmt.Errorf("failed to restart sing-box: %w", err)
		}
		if f.appLog != nil {
			f.appLog.Info("sing-box restart complete")
		}
	}

	f.lastFlush = time.Now()
	return nil
}

func noWindow(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func (f *DNSFlusher) FlushSystemDNS() error {
	switch runtime.GOOS {
	case "windows":
		return noWindow("ipconfig", "/flushdns").Run()
	case "darwin":
		if err := noWindow("dscacheutil", "-flushcache").Run(); err != nil {
			return err
		}
		return noWindow("killall", "-HUP", "mDNSResponder").Run()
	case "linux":
		if err := noWindow("systemd-resolve", "--flush-caches").Run(); err != nil {
			return noWindow("resolvectl", "flush-caches").Run()
		}
		return nil
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (f *DNSFlusher) GetLastFlushTime() time.Time {
	f.flushLock.Lock(); defer f.flushLock.Unlock()
	return f.lastFlush
}

func (f *DNSFlusher) CanFlush() bool {
	f.flushLock.Lock(); defer f.flushLock.Unlock()
	return time.Since(f.lastFlush) >= 10*time.Second
}
