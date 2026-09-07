//go:build !windows

package main

import (
	"fmt"
	"os"
)

func singBoxExecutableName() string {
	return "sing-box"
}

func validateInputConfig(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("%s is accessible by group or others; run chmod 600 %q", path, path)
	}
	return nil
}
