package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func init() {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(executable), ".oneproxy-upgrading")); err == nil {
		fmt.Fprintln(os.Stderr, "OneProxy is being upgraded. Wait for Setup to finish before launching it.")
		os.Exit(1)
	}
}
