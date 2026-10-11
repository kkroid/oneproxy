package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kkroid/oneproxy/internal/config"
	"testing"
	"time"

	"github.com/kkroid/oneproxy/internal/supervisor"
)

const helperProcessEnv = "ONEPROXY_MANAGER_HELPER"

func TestMain(m *testing.M) {
	if code, handled := supervisor.Main(); handled {
		os.Exit(code)
	}
	if os.Getenv(helperProcessEnv) == "1" {
		runManagerHelper()
		os.Exit(0)
	}
	info, err := os.Stat(os.Args[0])
	if err == nil {
		_ = os.Chmod(os.Args[0], info.Mode()|0100)
	}
	os.Exit(m.Run())
}

func runManagerHelper() {
	if callsPath := os.Getenv("ONEPROXY_MANAGER_CALLS"); callsPath != "" {
		file, err := os.OpenFile(callsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err == nil {
			_, _ = fmt.Fprintln(file, strings.Join(os.Args[1:], " "))
			_ = file.Close()
		}
	}
	switch os.Getenv("ONEPROXY_MANAGER_MODE") {
	case "exit":
		_, _ = fmt.Fprintln(os.Stderr, "listen tcp 127.0.0.1:12345: bind: address already in use")
		os.Exit(23)
	case "delay-exit":
		time.Sleep(100 * time.Millisecond)
		os.Exit(24)
	case "check-fail":
		if len(os.Args) > 1 && os.Args[1] == "check" {
			_, _ = fmt.Fprintln(os.Stderr, "invalid generated configuration")
			os.Exit(25)
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "run" {
		time.Sleep(time.Minute)
	}
}

func TestForegroundManagerRejectsEarlyExitWithoutRestart(t *testing.T) {
	manager, callsPath := newTestManager(t, false, "exit")
	manager.startupWindow = 500 * time.Millisecond

	if err := manager.Start(); err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("Start() error = %v, want early exit", err)
	}
	assertCallCount(t, callsPath, "run", 1)
}

func TestForegroundManagerReportsLaterExit(t *testing.T) {
	manager, callsPath := newTestManager(t, false, "delay-exit")
	manager.startupWindow = 20 * time.Millisecond

	if err := manager.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case err := <-manager.Done():
		if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
			t.Fatalf("Done() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal result")
	}
	assertCallCount(t, callsPath, "run", 1)
}

func TestForegroundManagerStopWaitsForReap(t *testing.T) {
	manager, _ := newTestManager(t, false, "sleep")
	manager.startupWindow = 20 * time.Millisecond
	done := manager.Done()

	if err := manager.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if manager.IsRunning() {
		t.Fatal("manager still reports running after Stop")
	}
	select {
	case err := <-done:
		t.Fatalf("intentional Stop reported terminal error: %v", err)
	default:
	}

	if err := manager.Start(); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if manager.Done() != done {
		t.Fatal("Done channel changed across process lifecycles")
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestDefaultManagerRetainsAutomaticRestart(t *testing.T) {
	manager, callsPath := newTestManager(t, true, "delay-exit")
	manager.startupWindow = 20 * time.Millisecond
	manager.restartDelay = 10 * time.Millisecond

	if err := manager.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case err := <-manager.Done():
		if err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
			t.Fatalf("Done() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for restart attempts")
	}
	assertCallCount(t, callsPath, "run", 3)
}

func TestManagerCheckUsesGeneratedConfig(t *testing.T) {
	manager, callsPath := newTestManager(t, false, "check-success")
	if err := manager.Check(); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	data, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "check --disable-color -c " + manager.configPath
	if strings.TrimSpace(string(data)) != want {
		t.Fatalf("check invocation = %q, want %q", strings.TrimSpace(string(data)), want)
	}
}

func TestManagerCheckReturnsChildOutput(t *testing.T) {
	manager, _ := newTestManager(t, false, "check-fail")
	if err := manager.Check(); err == nil || !strings.Contains(err.Error(), "invalid generated configuration") {
		t.Fatalf("Check() error = %v", err)
	}
}

func newTestManager(t *testing.T, autoRestart bool, mode string) (*Manager, string) {
	t.Helper()
	t.Setenv(helperProcessEnv, "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Setenv("ONEPROXY_MANAGER_MODE", mode)
	tempDir := t.TempDir()
	callsPath := filepath.Join(tempDir, "calls.log")
	t.Setenv("ONEPROXY_MANAGER_CALLS", callsPath)
	configPath := filepath.Join(tempDir, "generated.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(tempDir, "state", "logs", "singbox.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		t.Fatal(err)
	}
	manager := newManager(os.Args[0], configPath, logPath, autoRestart, 0)
	return manager, callsPath
}

func assertCallCount(t *testing.T, path, prefix string, want int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, prefix+" ") {
			count++
		}
	}
	if count != want {
		t.Fatalf("%s call count = %d, want %d; calls=%q", prefix, count, want, data)
	}
}

func TestDLLManagerRejectsEarlyExitWithDiagnostic(t *testing.T) {
	manager, calls := newTestManager(t, true, "exit")
	manager = NewManagerWithLog(manager.singboxPath, manager.configPath, manager.logPath)
	if err := os.WriteFile(manager.logPath, []byte("old unrelated failure\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := manager.Start()
	if err == nil || !strings.Contains(err.Error(), "address already in use") || !strings.Contains(err.Error(), "23") {
		t.Fatalf("Start() = %v, want child exit code and diagnostic", err)
	}
	if strings.Contains(err.Error(), "old unrelated failure") {
		t.Fatalf("stale diagnostic: %v", err)
	}
	if manager.IsRunning() || manager.GetPID() != 0 {
		t.Fatal("failed startup still running")
	}
	assertCallCount(t, calls, "run", 1)
}

func TestDLLManagerObservesStartupAndRejectsDuplicateStart(t *testing.T) {
	manager, calls := newTestManager(t, true, "sleep")
	manager = NewManagerWithLog(manager.singboxPath, manager.configPath, manager.logPath)
	t.Cleanup(func() { _ = manager.Stop() })
	start := time.Now()
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < defaultStartupWindow {
		t.Fatal("startup observation skipped")
	}
	if err := manager.Start(); err == nil {
		t.Fatal("duplicate Start succeeded")
	}
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Restart(); err == nil {
		t.Fatal("stale restart resurrected stopped manager")
	}
	assertCallCount(t, calls, "run", 1)
}

func TestManagerStopCancelsPendingAutomaticRestart(t *testing.T) {
	manager, calls := newTestManager(t, true, "delay-exit")
	manager.startupWindow = 20 * time.Millisecond
	manager.restartDelay = time.Hour
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	// Observe the actual process exit before stopping during restart backoff.
	deadline := time.Now().Add(3 * time.Second)
	for manager.IsRunning() {
		if time.Now().After(deadline) {
			t.Fatal("helper did not exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if manager.IsRunning() {
		t.Fatal("still running after stop")
	}
	assertCallCount(t, calls, "run", 1)
}

func TestManagerConcurrentRestartAndStopDoNotResurrect(t *testing.T) {
	manager, calls := newTestManager(t, true, "sleep")
	manager.startupWindow = 20 * time.Millisecond
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	waitForHelperCall(t, calls)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = manager.Restart() }()
	}
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if manager.IsRunning() {
		t.Fatal("restart resurrected manager after Stop")
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	before := string(data)
	if err := manager.Restart(); err == nil {
		t.Fatal("restart after Stop succeeded")
	}
	data, err = os.ReadFile(calls)
	if err != nil || string(data) != before {
		t.Fatal("stale recovery spawned a process")
	}
}

func TestDLLStopOrderStopsHealthChecker(t *testing.T) {
	manager, calls := newTestManager(t, true, "sleep")
	manager.startupWindow = 20 * time.Millisecond
	cfg := &config.Config{}
	cfg.HealthCheck.Enabled = true
	hc := NewHealthChecker(cfg, manager)
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	waitForHelperCall(t, calls)
	hc.Start()
	// Match OneProxy_Stop: stop health checks before stopping the child.
	hc.Stop()
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if hc.IsRunning() || manager.IsRunning() {
		t.Fatal("Stop left lifecycle active")
	}
	assertCallCount(t, calls, "run", 1)
}

func waitForHelperCall(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "run ") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not record run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
