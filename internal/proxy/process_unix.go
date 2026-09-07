//go:build darwin || linux

package proxy

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

type platformProcess struct {
	stopTimeout time.Duration
}

func configureProcessCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func (p *platformProcess) start(cmd *exec.Cmd) error {
	return cmd.Start()
}

func (p *platformProcess) stop(cmd *exec.Cmd, done <-chan struct{}) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := signalProcessGroup(cmd.Process.Pid, syscall.SIGTERM); err != nil {
		return err
	}
	timeout := p.stopTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if waitForLifecycle(done, timeout) {
		return nil
	}
	if err := signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return err
	}
	if !waitForLifecycle(done, timeout) {
		return fmt.Errorf("sing-box did not exit after SIGKILL")
	}
	return nil
}

func (p *platformProcess) close() {}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	err := syscall.Kill(-pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func waitForLifecycle(done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}
