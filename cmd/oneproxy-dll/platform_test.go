package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestPlatformFileNames(t *testing.T) {
	wantLibrary := "liboneproxy.so"
	wantSingBox := "sing-box"
	if runtime.GOOS == "windows" {
		wantLibrary = "oneproxy.dll"
		wantSingBox = "sing-box.exe"
	} else if runtime.GOOS == "darwin" {
		wantLibrary = "liboneproxy.dylib"
	}

	if got := sharedLibraryName(); got != wantLibrary {
		t.Fatalf("sharedLibraryName() = %q, want %q", got, wantLibrary)
	}
	if got := singBoxExecutableName(); got != wantSingBox {
		t.Fatalf("singBoxExecutableName() = %q, want %q", got, wantSingBox)
	}
	if runtime.GOOS != "windows" && loadRouteModeOverride() != "" {
		t.Fatal("loadRouteModeOverride() must be empty outside Windows")
	}
}

func TestDataDirWithEmptyHomeIsAbsolute(t *testing.T) {
	dir := dataDirForHome("")
	if !filepath.IsAbs(dir) {
		t.Fatalf("dataDirForHome(\"\") = %q, want absolute path", dir)
	}
	if filepath.Base(dir) != ".oneproxy" {
		t.Fatalf("dataDirForHome(\"\") = %q, want .oneproxy directory", dir)
	}
}
