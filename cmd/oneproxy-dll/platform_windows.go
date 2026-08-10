//go:build windows

package main

import "golang.org/x/sys/windows/registry"

func sharedLibraryName() string {
	return "oneproxy.dll"
}

func singBoxExecutableName() string {
	return "sing-box.exe"
}

func loadRouteModeOverride() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\OneProxy`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	routeMode, _, err := key.GetStringValue("RouteMode")
	if err != nil {
		return ""
	}
	return routeMode
}
