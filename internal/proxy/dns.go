package proxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/kkroid/oneproxy/internal/logger"
)

// DNSFlusher handles DNS cache flushing
type DNSFlusher struct {
	lastFlush     time.Time
	lastAttempt   time.Time
	flushLock     sync.Mutex
	flushCooldown time.Duration
	appLog        *logger.Logger
	flushSystem   func() error
	restart       func() error
}

func NewDNSFlusher() *DNSFlusher {
	return &DNSFlusher{
		flushCooldown: 10 * time.Second,
	}
}

// SetCooldown sets the minimum interval between flushes.
func (f *DNSFlusher) SetCooldown(d time.Duration) {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()
	f.flushCooldown = d
}

// SetLogger sets the logger for recording errors.
func (f *DNSFlusher) SetLogger(l *logger.Logger) {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()
	f.appLog = l
}

// FlushAll explicitly flushes the system DNS cache and restarts the core to
// clear its cache. Callers must not infer DNS cache failure from all-down checks.
func (f *DNSFlusher) FlushAll(manager *Manager) error {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()

	if manager == nil || !manager.IsRunning() {
		return fmt.Errorf("DNS recovery skipped: sing-box is not running")
	}

	if elapsed := time.Since(f.lastAttempt); elapsed < f.flushCooldown {
		if f.appLog != nil {
			f.appLog.Warn("DNS flush skipped — cooldown (%v remain)", (f.flushCooldown - elapsed).Round(time.Second))
		}
		return fmt.Errorf("DNS flush called too frequently, please wait")
	}

	// Failed attempts also consume the cooldown to avoid repeated system changes.
	f.lastAttempt = time.Now()
	flush := f.flushSystem
	if flush == nil {
		flush = flushSystemDNS
	}
	if err := flush(); err != nil {
		if f.appLog != nil {
			f.appLog.Error("DNS flush failed: %v", err)
		}
		return fmt.Errorf("failed to flush system DNS: %w", err)
	}
	f.lastFlush = time.Now()

	if !manager.IsRunning() {
		err := fmt.Errorf("system DNS flushed, but sing-box stopped or is not ready")
		if f.appLog != nil {
			f.appLog.Error("DNS recovery incomplete: %v", err)
		}
		return err
	}
	if f.appLog != nil {
		f.appLog.Info("DNS flushed — restarting sing-box")
	}
	restart := f.restart
	if restart == nil {
		restart = manager.Restart
	}
	if err := restart(); err != nil {
		if f.appLog != nil {
			f.appLog.Error("sing-box restart failed: %v", err)
		}
		return fmt.Errorf("failed to restart sing-box: %w", err)
	}
	if f.appLog != nil {
		f.appLog.Info("sing-box restart complete")
	}

	return nil
}

func (f *DNSFlusher) FlushSystemDNS() error {
	return flushSystemDNS()
}

// GetLastFlushTime reports system cache success, not core restart success.
func (f *DNSFlusher) GetLastFlushTime() time.Time {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()
	return f.lastFlush
}

func (f *DNSFlusher) CanFlush() bool {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()
	return time.Since(f.lastAttempt) >= f.flushCooldown
}
