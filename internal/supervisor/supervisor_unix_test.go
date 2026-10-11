//go:build darwin || linux

package supervisor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if code, handled := Main(); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestOwnerPipeClosureReapsChild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command("/bin/sh", "-c", `echo $$ > "$1"; exec sleep 30`, "fixture", pidFile)
	owner, err := Wrap(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	owner.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("supervisor exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		// Only terminate this test's supervisor and its recorded child.
		_ = cmd.Process.Kill()
		if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		t.Fatal("supervisor did not reap child after owner pipe closed")
	}
	if pid == 0 {
		t.Fatal("child did not start")
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("child remains after supervisor exit: %v", err)
	}
}
