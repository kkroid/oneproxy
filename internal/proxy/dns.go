package proxy

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// DNSFlusher handles DNS cache flushing
type DNSFlusher struct {
	lastFlush time.Time
	flushLock sync.Mutex
}

// NewDNSFlusher creates a new DNS flusher
func NewDNSFlusher() *DNSFlusher {
	return &DNSFlusher{}
}

// FlushAll performs a complete DNS flush: system cache + sing-box restart
func (f *DNSFlusher) FlushAll(manager *Manager) error {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()

	// Prevent frequent flushing (minimum 10 seconds between flushes)
	if time.Since(f.lastFlush) < 10*time.Second {
		return fmt.Errorf("DNS flush called too frequently, please wait")
	}

	// Step 1: Flush system DNS cache
	if err := f.FlushSystemDNS(); err != nil {
		return fmt.Errorf("failed to flush system DNS: %w", err)
	}

	// Step 2: Restart sing-box to clear internal DNS cache
	if manager != nil && manager.IsRunning() {
		if err := manager.Restart(); err != nil {
			return fmt.Errorf("failed to restart sing-box: %w", err)
		}
	}

	f.lastFlush = time.Now()
	return nil
}

// FlushSystemDNS flushes the system DNS cache
func (f *DNSFlusher) FlushSystemDNS() error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Windows: ipconfig /flushdns
		cmd = exec.Command("ipconfig", "/flushdns")

	case "darwin":
		// macOS: dscacheutil -flushcache; sudo killall -HUP mDNSResponder
		cmd = exec.Command("dscacheutil", "-flushcache")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("dscacheutil failed: %w", err)
		}
		// Also restart mDNSResponder (requires sudo, may fail)
		cmd = exec.Command("killall", "-HUP", "mDNSResponder")

	case "linux":
		// Linux: systemd-resolve --flush-caches (systemd-resolved)
		cmd = exec.Command("systemd-resolve", "--flush-caches")
		if err := cmd.Run(); err != nil {
			// Try alternative command: resolvectl flush-caches
			cmd = exec.Command("resolvectl", "flush-caches")
		}

	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("DNS flush command failed: %w", err)
	}

	return nil
}

// GetLastFlushTime returns the last flush time
func (f *DNSFlusher) GetLastFlushTime() time.Time {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()
	return f.lastFlush
}

// CanFlush returns whether enough time has passed since last flush
func (f *DNSFlusher) CanFlush() bool {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()
	return time.Since(f.lastFlush) >= 10*time.Second
}
