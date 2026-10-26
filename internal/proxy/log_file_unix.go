//go:build !windows

package proxy

import "os"

func openLogFile(path string) (*os.File, error) { return os.Open(path) }
