//go:build darwin || linux

// Package supervisor keeps Unix proxy children tied to their owner's lifetime.
package supervisor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const argument = "--internal-supervise"

var executable string

// Main must run before CLI argument parsing (also in subprocess test helpers).
// The tray uses the CLI installed beside it as its supervisor.
func Main() (int, bool) {
	executable, _ = os.Executable()
	if len(os.Args) < 2 || os.Args[1] != argument {
		return 0, false
	}
	if len(os.Args) < 3 {
		return 2, true
	}
	owner := os.NewFile(3, "owner-lifetime")
	if owner == nil {
		return 2, true
	}
	defer owner.Close()
	if info, err := owner.Stat(); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		return 2, true
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(signals)
	ownerGone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, owner)
		close(ownerGone)
	}()
	child := exec.Command(os.Args[2], os.Args[3:]...)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start sing-box:", err)
		return 1, true
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		return exitCode(err), true
	case <-ownerGone:
	case <-signals:
	}
	// The pipe also closes when the owner crashes or receives SIGKILL.
	// Only this supervisor's child group is signalled, never a name or port.
	_ = syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
		<-done
	}
	return 0, true
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(*exec.ExitError); ok && e.ExitCode() >= 0 {
		return e.ExitCode()
	}
	return 1
}

// Wrap replaces cmd with the supervisor and returns the owner's pipe end.
// Only the read end is inherited; sing-box never inherits either end.
func Wrap(cmd *exec.Cmd) (*os.File, error) {
	helper := executable
	if helper == "" {
		owner, err := os.Executable()
		if err != nil {
			return nil, err
		}
		helper = filepath.Join(filepath.Dir(owner), "oneproxy")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	args := append([]string{helper, argument, cmd.Path}, cmd.Args[1:]...)
	cmd.Path, cmd.Args = helper, args
	cmd.ExtraFiles = []*os.File{reader}
	err = cmd.Start()
	_ = reader.Close()
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	return writer, nil
}
