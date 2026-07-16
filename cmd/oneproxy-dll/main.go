package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

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

//export OneProxy_Start
func OneProxy_Start(configPath *C.char) *C.char {
	gMu.Lock()
	defer gMu.Unlock()

	cfg, err := config.Load(C.GoString(configPath))
	if err != nil { return errStr(err) }
	gConfig = cfg

	dataDir := resolveDataDir()
	genCfg := filepath.Join(dataDir, "singbox_generated.json")
	gen := config.NewSingBoxGenerator(cfg)
	if err := gen.SaveToFile(genCfg); err != nil { return errStr(err) }

	// sing-box binary lives next to the DLL/EXE, not in dataDir
	ab, _ := filepath.Abs("bin/sing-box.exe")
	if _, err := os.Stat(ab); os.IsNotExist(err) {
		return errStr(fmt.Errorf("sing-box.exe not found"))
	}

	manager := proxy.NewManagerWithLog(ab, genCfg, filepath.Join(dataDir, "logs", "singbox.log"))
	gManager = manager
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
