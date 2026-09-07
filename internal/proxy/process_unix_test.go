//go:build darwin || linux

package proxy

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestConfigureProcessCommandUnix(t *testing.T) {
	cmd := exec.Command("unused")
	configureProcessCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Error("child process is not assigned to a separate process group")
	}
}

func TestPlatformProcessStopWithoutCommandUnix(t *testing.T) {
	var process platformProcess
	if err := process.stop(nil, nil); err != nil {
		t.Fatalf("stop(nil) returned error: %v", err)
	}
}

func TestPlatformProcessStopEscalatesToSIGKILLUnix(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command("sh", "-c", "trap '' TERM; : > \"$1\"; while :; do sleep 1; done", "sh", ready)
	configureProcessCommand(cmd)
	process := platformProcess{stopTimeout: 100 * time.Millisecond}
	if err := process.start(cmd); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 100; attempt++ {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatalf("helper did not become ready: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	started := time.Now()
	if err := process.stop(cmd, done); err != nil {
		t.Fatalf("stop() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < 80*time.Millisecond {
		t.Fatalf("stop returned before graceful timeout: %v", elapsed)
	}
	if err := syscall.Kill(-cmd.Process.Pid, 0); err == nil {
		t.Fatalf("process group %d still exists", cmd.Process.Pid)
	}
}
