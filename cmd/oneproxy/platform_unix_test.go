//go:build darwin || linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateInputConfigUnixPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateInputConfig(path); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("validateInputConfig() error = %v, want chmod guidance", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateInputConfig(path); err != nil {
		t.Fatalf("validateInputConfig() error = %v", err)
	}
}
