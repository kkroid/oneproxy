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
	"github.com/kkroid/oneproxy/internal/logger"
	"github.com/kkroid/oneproxy/internal/proxy"
)

var (
	gManager       *proxy.Manager
	gHealthChecker *proxy.HealthChecker
	gDNSFlusher    *proxy.DNSFlusher
	gConfig        *config.Config
	gLogger        *logger.Logger
	gMu            sync.Mutex
)

// ---- helpers ----

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

// exeDir returns the directory containing oneproxy.dll and the tray EXE.
// The tray already calls SetCurrentDirectory to the correct location in
// loadDLL(), so cwd is reliable in production. For standalone/test use,
// we check multiple locations.
func exeDir() string {
	// Primary: cwd (tray sets this via SetCurrentDirectory in loadDLL)
	cwd, err := os.Getwd()
	if err == nil {
		if _, err := os.Stat(filepath.Join(cwd, "oneproxy.dll")); err == nil {
			return cwd
		}
	}

	// Fallback: host EXE dir (works when built standalone, not DLL)
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, "oneproxy.dll")); err == nil {
			return dir
		}
		// In test scenarios (Python), check if bin/ exists in EXE dir
		if _, err := os.Stat(filepath.Join(dir, "bin", "sing-box.exe")); err == nil {
			return dir
		}
	}

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
	if err != nil {
		// Fresh install or config deleted — copy placeholder from exe dir
		placeholder := filepath.Join(exeDir(), "config-placeholder.json")
		target := filepath.Join(resolveDataDir(), "config.json")
		if src, e := os.ReadFile(placeholder); e == nil {
			if e := os.WriteFile(target, src, 0644); e == nil {
				found = target
				err = nil
			}
		}
		if err != nil {
			return errStr(fmt.Errorf("config not found and no placeholder available"))
		}
	}

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

	// Init application logger (10 MB max, keep 3 rotated backups)
	if gLogger == nil {
		gLogger, _ = logger.New(filepath.Join(dataDir, "logs", "oneproxy.log"), 10, 3)
	}
	if gLogger != nil {
		gLogger.Info("OneProxy v0.5.0 starting, route=%s, proxies=%d, port=%d",
			cfg.RouteMode, len(cfg.GetEnabledProxies()), cfg.Unified.Port)
	}

	genCfg := filepath.Join(dataDir, "singbox_generated.json")
	ed := exeDir()

	gen := config.NewSingBoxGenerator(cfg, ed)
	if err := gen.SaveToFile(genCfg); err != nil { return errStr(err) }

	// sing-box binary - try cwd/bin/ first, then exe dir/bin/
	cwd, _ := filepath.Abs(".")

	for _, dir := range []string{cwd, ed} {
		ab := filepath.Join(dir, "bin", "sing-box.exe")
		ab, _ = filepath.Abs(ab)
		if _, err := os.Stat(ab); err == nil {
			manager := proxy.NewManagerWithLog(ab, genCfg, filepath.Join(dataDir, "logs", "singbox.log"))
			manager.SetLogger(gLogger)
			gManager = manager
			goto started
		}
	}
	return errStr(fmt.Errorf("sing-box.exe not found (cwd=%s, exe=%s)", cwd, ed))

started:
	gHealthChecker = proxy.NewHealthChecker(cfg, gManager)
	gHealthChecker.SetLogger(gLogger)
	gDNSFlusher = proxy.NewDNSFlusher()

	gHealthChecker.SetFailureCallback(func(name string) {
		gDNSFlusher.FlushAll(gManager)
	})

	if err := gManager.Start(); err != nil { return errStr(err) }
	if cfg.HealthCheck.Enabled {
		if gLogger != nil {
			gLogger.Info("health check started, interval=%ds, timeout=%ds",
				cfg.HealthCheck.IntervalSeconds, cfg.HealthCheck.TimeoutSeconds)
		}
		gHealthChecker.Start()
	}
	if gLogger != nil {
		gLogger.Info("started OK")
	}
	return nil
}

//export OneProxy_Stop
func OneProxy_Stop() *C.char {
	gMu.Lock()
	defer gMu.Unlock()
	if gLogger != nil { gLogger.Info("stopping") }
	if gHealthChecker != nil { gHealthChecker.Stop() }
	if gManager != nil { gManager.Stop() }
	gManager, gHealthChecker, gDNSFlusher, gConfig = nil, nil, nil, nil
	if gLogger != nil { gLogger.Info("stopped") }
	return nil
}

//export OneProxy_Restart
func OneProxy_Restart() *C.char {
	gMu.Lock()
	defer gMu.Unlock()
	if gManager == nil { return errStr(fmt.Errorf("not started")) }
	if gLogger != nil { gLogger.Info("restarting") }
	if gHealthChecker != nil { gHealthChecker.Stop() }
	gManager.Restart()
	if gConfig != nil && gConfig.HealthCheck.Enabled && gHealthChecker != nil { gHealthChecker.Start() }
	if gLogger != nil { gLogger.Info("restarted") }
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

// reloadAndRestart regenerates singbox_generated.json from gConfig and restarts
// sing-box. Must be called with gMu held.
func reloadAndRestart() error {
	if gConfig == nil || gManager == nil || !gManager.IsRunning() {
		return nil
	}
	ed := exeDir()
	genCfg := filepath.Join(resolveDataDir(), "singbox_generated.json")
	gen := config.NewSingBoxGenerator(gConfig, ed)
	if err := gen.SaveToFile(genCfg); err != nil {
		return err
	}
	gManager.SetConfigPath(genCfg)
	if gHealthChecker != nil {
		gHealthChecker.Stop()
		gHealthChecker.SetConfig(gConfig)
	}
	gManager.Restart()
	if gConfig.HealthCheck.Enabled && gHealthChecker != nil {
		gHealthChecker.Start()
	}
	return nil
}

//export OneProxy_ExportConfig
func OneProxy_ExportConfig() *C.char {
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
func OneProxy_ImportConfig(input *C.char) *C.char {
	raw := C.GoString(input)

	// Smart detection: URL or base64 backup?
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		// ─── Subscription URL ──────────────────────────────
		proxies, subURL, err := config.FetchSubscription(raw, 10801)
		if err != nil {
			return errStr(fmt.Errorf("fetch subscription: %w", err))
		}

		// Load existing config to preserve other settings
		var cfg *config.Config
		cfgPath, pathErr := resolveConfig("config.json")
		if pathErr == nil {
			cfg, _ = config.Load(cfgPath)
		}
		if cfg == nil {
			cfg = &config.Config{Version: "1.0"}
		}
		cfg.Proxies = proxies
		cfg.SubscriptionURL = subURL

		savePath := filepath.Join(resolveDataDir(), "config.json")
		if err := cfg.Save(savePath); err != nil {
			return errStr(fmt.Errorf("save config: %w", err))
		}

		// Replace running config, regenerate + restart
		gMu.Lock()
		gConfig = cfg
		_ = reloadAndRestart()
		gMu.Unlock()
		return nil
	}

	// ─── Base64 backup ──────────────────────────────────
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return errStr(fmt.Errorf("not a valid URL or base64 backup"))
	}
	var tmp interface{}
	if err := json.Unmarshal(decoded, &tmp); err != nil {
		return errStr(fmt.Errorf("not valid JSON"))
	}

	savePath := filepath.Join(resolveDataDir(), "config.json")
	if err := os.WriteFile(savePath, decoded, 0644); err != nil {
		return errStr(fmt.Errorf("cannot write config: %w", err))
	}

		// Reload, regenerate + restart
		gMu.Lock()
		cfg, _ := config.Load(savePath)
		if cfg != nil {
			gConfig = cfg
		}
		_ = reloadAndRestart()
		gMu.Unlock()
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
func OneProxy_GetVersion() *C.char { return C.CString("0.5.0") }

//export OneProxy_FreeString
func OneProxy_FreeString(s *C.char) { C.free(unsafe.Pointer(s)) }

func main() {}
