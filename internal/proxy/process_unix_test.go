//go:build darwin || linux

package proxy

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	cmd := exec.Command("sh", "-c", "trap '' TERM; echo $$ > \"$1.tmp\"; mv \"$1.tmp\" \"$1\"; exec sleep 30", "sh", ready)
	configureProcessCommand(cmd)
	process := platformProcess{stopTimeout: 5 * time.Second}
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
	data, err := os.ReadFile(ready)
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || childPID <= 0 {
		t.Fatalf("invalid child PID: %q", data)
	}
	started := time.Now()
	if err := process.stop(cmd, done); err != nil {
		t.Fatalf("stop() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < 1800*time.Millisecond {
		t.Fatalf("stop returned before graceful timeout: %v", elapsed)
	}
	if !cmd.ProcessState.Success() {
		t.Fatalf("supervisor exit = %v, want successful cleanup", cmd.ProcessState)
	}
	if err := syscall.Kill(childPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("child %d: signal 0 returned %v, want ESRCH", childPID, err)
	}
}
