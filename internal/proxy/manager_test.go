package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const helperProcessEnv = "ONEPROXY_MANAGER_HELPER"

func TestMain(m *testing.M) {
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
	manager, callsPath := newTestManager(t, true, "exit")
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
