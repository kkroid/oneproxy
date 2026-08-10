//go:build !windows

package main

import "runtime"

func sharedLibraryName() string {
	if runtime.GOOS == "darwin" {
		return "liboneproxy.dylib"
	}
	return "liboneproxy.so"
}

func singBoxExecutableName() string {
	return "sing-box"
}

func loadRouteModeOverride() string {
	return ""
}
