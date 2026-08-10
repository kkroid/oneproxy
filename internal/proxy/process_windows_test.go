//go:build windows

package proxy

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigureProcessCommandWindows(t *testing.T) {
	cmd := exec.Command("unused")
	configureProcessCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("child process window is not hidden")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Error("CREATE_NO_WINDOW flag is not set")
	}
}

func TestPlatformProcessStopWithoutCommandWindows(t *testing.T) {
	var process platformProcess
	if err := process.stop(nil); err != nil {
		t.Fatalf("stop(nil) returned error: %v", err)
	}
}
