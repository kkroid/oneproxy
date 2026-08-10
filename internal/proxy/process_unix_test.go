//go:build darwin || linux

package proxy

import (
	"os/exec"
	"testing"
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
	if err := process.stop(nil); err != nil {
		t.Fatalf("stop(nil) returned error: %v", err)
	}
}
