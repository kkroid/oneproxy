package main

import (
	"runtime"
	"testing"
)

func TestSingBoxExecutableName(t *testing.T) {
	want := "sing-box"
	if runtime.GOOS == "windows" {
		want = "sing-box.exe"
	}
	if got := singBoxExecutableName(); got != want {
		t.Fatalf("singBoxExecutableName() = %q, want %q", got, want)
	}
}
