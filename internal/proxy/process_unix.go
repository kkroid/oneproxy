//go:build darwin || linux

package proxy

import (
	"errors"
	"os/exec"
	"syscall"
)

type platformProcess struct{}

func configureProcessCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func (p *platformProcess) start(cmd *exec.Cmd) error {
	return cmd.Start()
}

func (p *platformProcess) stop(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (p *platformProcess) close() {}
