package proxy

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDNSRecoveryRejectsInactiveCore(t *testing.T) {
	for _, tt := range []struct {
		name    string
		manager *Manager
	}{
		{"nil", nil},
		{"not started", NewManager("missing", "missing")},
		{"startup observation", &Manager{isRunning: true}},
		{"configuration early exit", &Manager{}},
		{"port conflict early exit", &Manager{}},
		{"exited awaiting restart", &Manager{isRunning: true, ready: false}},
		{"restart observation", &Manager{isRunning: true, ready: false}},
		{"stopping", &Manager{isRunning: true, ready: true, stopping: true}},
		{"stopped", &Manager{isRunning: false, ready: false}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDNSFlusher()
			f.restart = func() error {
				t.Fatal("inactive core must not restart")
				return nil
			}
			f.flushSystem = func() error {
				t.Fatal("inactive core must not flush system DNS")
				return nil
			}
			if err := f.FlushAll(tt.manager); err == nil {
				t.Fatal("expected inactive core error")
			}
			if !f.CanFlush() || !f.GetLastFlushTime().IsZero() {
				t.Fatal("skipped recovery changed flush state")
			}
		})
	}
}

func TestDNSRecoveryFailureConsumesCooldown(t *testing.T) {
	f := NewDNSFlusher()
	f.SetCooldown(time.Hour)
	want := errors.New("API unavailable")
	calls := 0
	f.flushSystem = func() error { calls++; return want }
	f.restart = func() error {
		t.Fatal("failed system flush must not restart core")
		return nil
	}
	manager := &Manager{isRunning: true, ready: true}
	if err := f.FlushAll(manager); !errors.Is(err, want) {
		t.Fatalf("FlushAll() = %v, want API error", err)
	}
	if f.CanFlush() || !f.GetLastFlushTime().IsZero() {
		t.Fatal("failed attempt must consume cooldown without recording success")
	}
	if err := f.FlushAll(manager); err == nil || calls != 1 {
		t.Fatalf("second attempt = %v, calls = %d", err, calls)
	}
	f.SetCooldown(0)
	if !f.CanFlush() {
		t.Fatal("CanFlush ignored configured cooldown")
	}
}

func TestDNSRecoveryDoesNotRestartCoreStoppedDuringFlush(t *testing.T) {
	manager := &Manager{isRunning: true, ready: true}
	f := NewDNSFlusher()
	calls := 0
	f.restart = func() error {
		t.Fatal("core stopped during flush must not restart")
		return nil
	}
	f.flushSystem = func() error {
		calls++
		manager.mutex.Lock()
		// A crashed core loses readiness before its lifecycle finishes.
		manager.ready = false
		manager.mutex.Unlock()
		return nil
	}
	if err := f.FlushAll(manager); err == nil || !strings.Contains(err.Error(), "system DNS flushed, but sing-box stopped or is not ready") {
		t.Fatalf("FlushAll() = %v, want partial completion error", err)
	}
	if calls != 1 || f.GetLastFlushTime().IsZero() || f.CanFlush() {
		t.Fatal("successful flush did not record success and cooldown")
	}
}

func TestDNSManualFlushRestartResult(t *testing.T) {
	restartFailure := errors.New("restart failed")
	for _, want := range []error{nil, restartFailure} {
		name := "success"
		if want != nil {
			name = "restart failure"
		}
		t.Run(name, func(t *testing.T) {
			f := NewDNSFlusher()
			var calls []string
			f.flushSystem = func() error { calls = append(calls, "flush"); return nil }
			f.restart = func() error { calls = append(calls, "restart"); return want }
			err := f.FlushAll(&Manager{isRunning: true, ready: true})
			if !errors.Is(err, want) {
				t.Fatalf("FlushAll() = %v, want %v", err, want)
			}
			if want != nil && !strings.Contains(err.Error(), "failed to restart sing-box") {
				t.Fatalf("missing restart error context: %v", err)
			}
			if strings.Join(calls, ",") != "flush,restart" {
				t.Fatalf("operation order = %v", calls)
			}
			if f.GetLastFlushTime().IsZero() || f.CanFlush() {
				t.Fatal("system success must record lastFlush and consume cooldown even if restart fails")
			}
			if err := f.FlushAll(&Manager{isRunning: true, ready: true}); err == nil || len(calls) != 2 {
				t.Fatalf("cooldown failed: err=%v, calls=%v", err, calls)
			}
		})
	}
}

func TestDNSRecoveryConcurrentFailureIsThrottled(t *testing.T) {
	f := NewDNSFlusher()
	calls := 0
	f.flushSystem = func() error { calls++; return errors.New("API unavailable") }
	manager := &Manager{isRunning: true, ready: true}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			f.SetCooldown(time.Hour)
			f.SetLogger(nil)
			_ = f.FlushAll(manager)
			_ = f.CanFlush()
			_ = f.GetLastFlushTime()
		})
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("system flush attempts = %d, want 1", calls)
	}
}
