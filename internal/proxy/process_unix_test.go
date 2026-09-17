//go:build darwin || linux

package proxy

import (
	"errors"
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
	// exec keeps a single process that ignores SIGTERM. A shell spawning sleep
	// leaves an orphan after group SIGKILL; its reaping depends on the host init.
	cmd := exec.Command("sh", "-c", "trap '' TERM; : > \"$1\"; exec sleep 30", "sh", ready)
	configureProcessCommand(cmd)
	process := platformProcess{stopTimeout: 100 * time.Millisecond}
	if err := process.start(cmd); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
			_ = signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
	})
	for attempt := 0; attempt < 100; attempt++ {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatalf("helper did not become ready: %v", err)
	}
	started := time.Now()
	if err := process.stop(cmd, done); err != nil {
		t.Fatalf("stop() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < 80*time.Millisecond {
		t.Fatalf("stop returned before graceful timeout: %v", elapsed)
	}
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("helper exit = %v, want SIGKILL", cmd.ProcessState)
	}
	if err := syscall.Kill(-cmd.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("process group %d: signal 0 returned %v, want ESRCH", cmd.Process.Pid, err)
	}
}
