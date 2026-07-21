package proxy

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// DNSFlusher handles DNS cache flushing
type DNSFlusher struct {
	lastFlush time.Time
	flushLock sync.Mutex
}

func NewDNSFlusher() *DNSFlusher { return &DNSFlusher{} }

func (f *DNSFlusher) FlushAll(manager *Manager) error {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()

	if time.Since(f.lastFlush) < 10*time.Second {
		return fmt.Errorf("DNS flush called too frequently, please wait")
	}

	if err := f.FlushSystemDNS(); err != nil {
		return fmt.Errorf("failed to flush system DNS: %w", err)
	}

	if manager != nil && manager.IsRunning() {
		if err := manager.Restart(); err != nil {
			return fmt.Errorf("failed to restart sing-box: %w", err)
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
