//go:build windows

package main

import (
	"fmt"
	"os"
)

func singBoxExecutableName() string {
	return "sing-box.exe"
}

func validateInputConfig(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}
