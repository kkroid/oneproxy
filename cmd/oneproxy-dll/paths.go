package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// resolveDataDir returns ~/.oneproxy/ as an absolute path and creates it.
func resolveDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	dir := dataDirForHome(home)
	_ = os.MkdirAll(filepath.Join(dir, "logs"), 0755)
	return dir
}

func dataDirForHome(home string) string {
	if home == "" {
		home, _ = os.Getwd()
	}
	dir, err := filepath.Abs(filepath.Join(home, ".oneproxy"))
	if err == nil {
		return dir
	}
	return filepath.Join(home, ".oneproxy")
}

// exeDir returns the directory containing the OneProxy shared library and tray app.
func exeDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd, _ = filepath.Abs(".")
	}
	if pathExists(filepath.Join(cwd, sharedLibraryName())) {
		return cwd
	}

	executable, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(executable)
		if pathExists(filepath.Join(dir, sharedLibraryName())) ||
			pathExists(filepath.Join(dir, "bin", singBoxExecutableName())) {
			return dir
		}
	}

	return cwd
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isMacOSBundleDir(dir string) bool {
	return runtime.GOOS == "darwin" && filepath.Base(dir) == "MacOS" &&
		filepath.Base(filepath.Dir(dir)) == "Contents"
}

// Bundled data belongs in Resources, outside the signed executable directory.
func resourceDir(dir string) string {
	if isMacOSBundleDir(dir) {
		return filepath.Join(filepath.Dir(dir), "Resources")
	}
	return dir
}

func singBoxPath(dir string) string {
	if isMacOSBundleDir(dir) {
		return filepath.Join(dir, singBoxExecutableName())
	}
	return filepath.Join(dir, "bin", singBoxExecutableName())
}
