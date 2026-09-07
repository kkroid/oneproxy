//go:build windows

package proxy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type platformProcess struct {
	job         windows.Handle
	mutex       sync.Mutex
	stopTimeout time.Duration
}

func configureProcessCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}

func (p *platformProcess) start(cmd *exec.Cmd) error {
	p.close()

	job, err := createJobObject()
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}
	p.mutex.Lock()
	p.job = job
	p.mutex.Unlock()

	if err := cmd.Start(); err != nil {
		p.close()
		return err
	}

	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = cmd.Process.Kill()
		p.close()
		return fmt.Errorf("open process handle: %w", err)
	}
	defer windows.CloseHandle(processHandle)

	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		_ = cmd.Process.Kill()
		p.close()
		return fmt.Errorf("assign process to job: %w", err)
	}

	return nil
}

func (p *platformProcess) stop(cmd *exec.Cmd, done <-chan struct{}) error {
	if cmd == nil || cmd.Process == nil {
		p.close()
		return nil
	}
	err := cmd.Process.Kill()
	p.close()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	timeout := p.stopTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("timed out waiting for sing-box to exit")
	}
}

func (p *platformProcess) close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.job != 0 {
		_ = windows.CloseHandle(p.job)
		p.job = 0
	}
}

func createJobObject() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}

	return job, nil
}
