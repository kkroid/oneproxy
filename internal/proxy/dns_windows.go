//go:build windows

package proxy

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func flushSystemDNS() error {
	cmd := exec.Command("ipconfig", "/flushdns")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	return cmd.Run()
}
