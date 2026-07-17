package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"github.com/kkroid/oneproxy/internal/config"
	"github.com/kkroid/oneproxy/internal/proxy"
)

var (
	gManager       *proxy.Manager
	gHealthChecker *proxy.HealthChecker
	gDNSFlusher    *proxy.DNSFlusher
	gConfig        *config.Config
	gMu            sync.Mutex
)

// ---- helpers ----

func copyFile(src, dst string) {
	if data, err := os.ReadFile(src); err == nil {
		os.WriteFile(dst, data, 0644)
	}
}

func errStr(err error) *C.char {
	if err == nil { return nil }
	return C.CString(err.Error())
}

// ---- exports ----

// resolveDataDir returns ~/.oneproxy/ as an absolute path and creates it.
// The tray EXE may have its cwd set to a read-only install directory, so we
// explicitly derive the path from the user profile and make it absolute.
func resolveDataDir() string {
	dir := filepath.Join(os.Getenv("USERPROFILE"), ".oneproxy")
	if s := os.Getenv("HOME"); dir == "" && s != "" {
		dir = filepath.Join(s, ".oneproxy")
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".oneproxy")
	}
	dir, _ = filepath.Abs(dir)
	os.MkdirAll(dir, 0755)
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)
	return dir
}

// exeDir returns the directory containing the host executable. In production
// this is the tray EXE's directory (which also contains config.json and the
// DLL), making it the right anchor for resolving relative asset paths.
func exeDir() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	// Fallback — shouldn't happen on any real Windows system
	dir, _ := filepath.Abs(".")
	return dir
}

// resolveConfig finds config.json in: 1) directly if absolute, 2) cwd,
// 3) exe/dll directory (production), 4) ~/.oneproxy/ (installed fallback).
func resolveConfig(configPath string) (string, error) {
	if filepath.IsAbs(configPath) {
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
	}

	// Build candidates: cwd, exe dir, user data dir
	cwd, _ := filepath.Abs(".")
	candidates := []string{
		filepath.Join(cwd, configPath),
		filepath.Join(exeDir(), configPath),
		filepath.Join(resolveDataDir(), configPath),
	}
	for _, p := range candidates {
		p, _ = filepath.Abs(p)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf(
		"config not found (tried cwd=%s, exe=%s, data=%s)",
		cwd, exeDir(), resolveDataDir(),
	)
}

//export OneProxy_Start
func OneProxy_Start(configPath *C.char) *C.char {
	gMu.Lock()
	defer gMu.Unlock()

	cp := C.GoString(configPath)
	found, err := resolveConfig(cp)
	if err != nil { return errStr(err) }

	cfg, err := config.Load(found)
	if err != nil { return errStr(err) }

	// Routing mode override from registry (set by tray menu)
	if key, err := registry.OpenKey(registry.CURRENT_USER, `Software\OneProxy`, registry.QUERY_VALUE); err == nil {
		if v, _, err := key.GetStringValue("RouteMode"); err == nil && v != "" {
			cfg.RouteMode = v
		}
		key.Close()
	}

	gConfig = cfg

	dataDir := resolveDataDir()
	genCfg := filepath.Join(dataDir, "singbox_generated.json")

	// Copy geoip/geosite to data dir so sing-box can find them
	ed := exeDir()
	for _, db := range []string{"geoip.db", "geosite.db"} {
		src := filepath.Join(ed, "bin", db)
		dst := filepath.Join(dataDir, db)
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			copyFile(src, dst)
		}
	}

	gen := config.NewSingBoxGenerator(cfg)
	if err := gen.SaveToFile(genCfg); err != nil { return errStr(err) }

	// sing-box binary — try cwd/bin/ first, then exe dir/bin/
	cwd, _ := filepath.Abs(".")
	
	for _, dir := range []string{cwd, ed} {
		ab := filepath.Join(dir, "bin", "sing-box.exe")
		ab, _ = filepath.Abs(ab)
		if _, err := os.Stat(ab); err == nil {
			manager := proxy.NewManagerWithLog(ab, genCfg, filepath.Join(dataDir, "logs", "singbox.log"))
			gManager = manager
			goto started
		}
	}
	return errStr(fmt.Errorf("sing-box.exe not found (cwd=%s, exe=%s)", cwd, ed))

started:
	gHealthChecker = proxy.NewHealthChecker(cfg, gManager)
	gDNSFlusher = proxy.NewDNSFlusher()

	gHealthChecker.SetFailureCallback(func(name string) {
		gDNSFlusher.FlushAll(gManager)
	})

	if err := gManager.Start(); err != nil { return errStr(err) }
	if cfg.HealthCheck.Enabled { gHealthChecker.Start() }
	fmt.Println("OneProxy: started")
	return nil
}

//export OneProxy_Stop
func OneProxy_Stop() *C.char {
	gMu.Lock()
	defer gMu.Unlock()
	if gHealthChecker != nil { gHealthChecker.Stop() }
	if gManager != nil { gManager.Stop() }
	gManager, gHealthChecker, gDNSFlusher, gConfig = nil, nil, nil, nil
	fmt.Println("OneProxy: stopped")
	return nil
}

//export OneProxy_Restart
func OneProxy_Restart() *C.char {
	gMu.Lock()
	defer gMu.Unlock()
	if gManager == nil { return errStr(fmt.Errorf("not started")) }
	if gHealthChecker != nil { gHealthChecker.Stop() }
	gManager.Restart()
	if gConfig != nil && gConfig.HealthCheck.Enabled && gHealthChecker != nil { gHealthChecker.Start() }
	return nil
}

type statusOut struct {
	Running     bool          `json:"running"`
	UnifiedPort int           `json:"unified_port"`
	Proxies     []statusProxy `json:"proxies"`
}

type statusProxy struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Type      string `json:"type"`
	Enabled   bool   `json:"enabled"`
	IsHealthy bool   `json:"is_healthy"`
	LatencyMS int64  `json:"latency_ms"`
}

//export OneProxy_Status
func OneProxy_Status() *C.char {
	gMu.Lock()
	defer gMu.Unlock()

	out := statusOut{}
	if gManager != nil { out.Running = gManager.IsRunning() }
	if gConfig != nil {
		out.UnifiedPort = gConfig.Unified.Port
		for _, p := range gConfig.Proxies {
			px := statusProxy{Name: p.Name, Port: p.LocalPort, Type: p.Type, Enabled: p.Enabled}
			if gHealthChecker != nil {
				if r := gHealthChecker.GetResult(p.Name); r != nil {
					px.IsHealthy = r.IsHealthy
					px.LatencyMS = r.Latency.Milliseconds()
				}
			}
			out.Proxies = append(out.Proxies, px)
		}
	}
	b, _ := json.Marshal(out)
	return C.CString(string(b))
}

//export OneProxy_HealthCheck
func OneProxy_HealthCheck() *C.char {
	if gHealthChecker == nil || gManager == nil || !gManager.IsRunning() {
		return errStr(fmt.Errorf("not running"))
	}
	gHealthChecker.CheckAll()
	return nil
}

//export OneProxy_FlushDNS
func OneProxy_FlushDNS() *C.char {
	if gDNSFlusher == nil || gManager == nil { return errStr(fmt.Errorf("not running")) }
	if !gDNSFlusher.CanFlush() { return errStr(fmt.Errorf("too frequent")) }
	gDNSFlusher.FlushAll(gManager)
	return nil
}

// sanitizeTag mirrors config.SingBoxGenerator.sanitizeTag so the tag we PUT
// to the Clash API matches the outbound tag sing-box actually registered.
func sanitizeTag(name string) string {
	var b strings.Builder
	for _, c := range name {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_':
			b.WriteRune(c)
		case c == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

//export OneProxy_ExportConfig
func OneProxy_ExportConfig() *C.char {
	// Use the same resolution order as Start: cwd → exeDir → dataDir
	path, err := resolveConfig("config.json")
	if err != nil {
		return errStr(fmt.Errorf("cannot find config: %w", err))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errStr(fmt.Errorf("cannot read config: %w", err))
	}
	return C.CString(base64.StdEncoding.EncodeToString(data))
}

//export OneProxy_ImportConfig
func OneProxy_ImportConfig(b64 *C.char) *C.char {
	raw, err := base64.StdEncoding.DecodeString(C.GoString(b64))
	if err != nil {
		return errStr(fmt.Errorf("invalid base64: %w", err))
	}
	var tmp interface{}
	if err := json.Unmarshal(raw, &tmp); err != nil {
		return errStr(fmt.Errorf("invalid JSON: %w", err))
	}
	// Always write to the user-writable data dir
	path := filepath.Join(resolveDataDir(), "config.json")
	if err := os.WriteFile(path, raw, 0644); err != nil {
		return errStr(fmt.Errorf("cannot write config: %w", err))
	}
	return nil
}

//export OneProxy_SelectProxy
func OneProxy_SelectProxy(proxyName *C.char) *C.char {
	gMu.Lock()
	cfg := gConfig
	gMu.Unlock()
	if cfg == nil || cfg.Unified.Port <= 0 {
		return errStr(fmt.Errorf("unified port not configured"))
	}
	selectorTag := cfg.Unified.Tag
	if selectorTag == "" { selectorTag = "proxy" }

	outboundTag := "out-" + sanitizeTag(C.GoString(proxyName))
	payload, _ := json.Marshal(map[string]string{"name": outboundTag})
	apiURL := fmt.Sprintf("http://127.0.0.1:9090/proxies/%s", selectorTag)

	req, err := http.NewRequest("PUT", apiURL, bytes.NewReader(payload))
	if err != nil { return errStr(err) }
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil { return errStr(err) }
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errStr(fmt.Errorf("clash api returned %d", resp.StatusCode))
	}
	return nil
}

//export OneProxy_GetVersion
func OneProxy_GetVersion() *C.char { return C.CString("0.4.0") }

//export OneProxy_FreeString
func OneProxy_FreeString(s *C.char) { C.free(unsafe.Pointer(s)) }

func main() {}
