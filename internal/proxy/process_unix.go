//go:build darwin || linux

package proxy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/kkroid/oneproxy/internal/supervisor"
)

type platformProcess struct {
	stopTimeout time.Duration
	mu          sync.Mutex
	owner       *os.File
}

func configureProcessCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func (p *platformProcess) start(cmd *exec.Cmd) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	owner, err := supervisor.Wrap(cmd)
	if err == nil {
		p.owner = owner
	}
	return err
}

func (p *platformProcess) stop(cmd *exec.Cmd, done <-chan struct{}) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	p.close()
	timeout := p.stopTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if waitForLifecycle(done, timeout) {
		return nil
	}
	return fmt.Errorf("sing-box supervisor did not finish cleanup")
}

func (p *platformProcess) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.owner != nil {
		_ = p.owner.Close()
		p.owner = nil
	}
}

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
