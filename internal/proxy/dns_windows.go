//go:build windows

package proxy

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

func flushSystemDNS() error {
	proc := windows.NewLazySystemDLL("dnsapi.dll").NewProc("DnsFlushResolverCache")
	return flushWindowsDNS(proc)
}

type dnsFlushProc interface {
	Find() error
	Call(...uintptr) (uintptr, uintptr, error)
}

func flushWindowsDNS(proc dnsFlushProc) error {
	if err := proc.Find(); err != nil {
		return fmt.Errorf("load DnsFlushResolverCache: %w", err)
	}
	result, _, err := proc.Call()
	if result == 0 {
		if err != nil && err != syscall.Errno(0) {
			return fmt.Errorf("DnsFlushResolverCache failed: %w", err)
		}
		return fmt.Errorf("DnsFlushResolverCache returned FALSE without an error code")
	}
	return nil
}
