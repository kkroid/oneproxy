//go:build darwin || linux

package proxy

import (
	"os/exec"
	"runtime"
)

func flushSystemDNS() error {
	if runtime.GOOS == "darwin" {
		if err := exec.Command("dscacheutil", "-flushcache").Run(); err != nil {
			return err
		}
		return exec.Command("killall", "-HUP", "mDNSResponder").Run()
	}

	if err := exec.Command("systemd-resolve", "--flush-caches").Run(); err != nil {
		return exec.Command("resolvectl", "flush-caches").Run()
	}
	return nil
}
