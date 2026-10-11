//go:build darwin || linux

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func flushSystemDNS() error {
	if runtime.GOOS == "darwin" {
		if os.Geteuid() != 0 {
			return fmt.Errorf("macOS system DNS flush requires administrator privileges; sing-box DNS cache can still be reset by restarting its core")
		}
		if err := runDNSCommand("/usr/bin/dscacheutil", "-flushcache"); err != nil {
			return err
		}
		return runDNSCommand("/usr/bin/killall", "-HUP", "mDNSResponder")
	}

	if err := runDNSCommand("resolvectl", "flush-caches"); err != nil {
		return runDNSCommand("systemd-resolve", "--flush-caches")
	}
	return nil
}

func runDNSCommand(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}
