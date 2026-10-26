package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/kkroid/oneproxy/internal/config"
	"github.com/kkroid/oneproxy/internal/logger"
	"github.com/kkroid/oneproxy/internal/proxy"
)

var appVersion = "0.8.3" // Windows release builds set this from installer metadata.

var (
	gManager       *proxy.Manager
	gHealthChecker *proxy.HealthChecker
	gDNSFlusher    *proxy.DNSFlusher
	gConfig        *config.Config
	gLogger        *logger.Logger
	gMu            sync.Mutex
	gStartup       *startupSession
	gConfigPath    string
	gStartEpoch    uint64
)

type startupSession struct {
	cancel context.CancelFunc
}

// ---- helpers ----

func errStr(err error) *C.char {
	if err == nil {
		return nil
	}
	return C.CString(err.Error())
}

// ---- exports ----

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
	return errStr(startProxy(C.GoString(configPath)))
}

// OneProxy_StartupEpoch must be called before dispatching asynchronous Start.
// Stop invalidates queued calls as well as canceling an active download.
//export OneProxy_StartupEpoch
func OneProxy_StartupEpoch() C.ulonglong {
	gMu.Lock()
	defer gMu.Unlock()
	return C.ulonglong(gStartEpoch)
}

//export OneProxy_StartAtEpoch
func OneProxy_StartAtEpoch(configPath *C.char, epoch C.ulonglong) *C.char {
	gMu.Lock()
	defer gMu.Unlock()
	return errStr(startProxyAtEpoch(C.GoString(configPath), uint64(epoch)))
}

// Requires gMu: the epoch check and startup registration are one operation.
func startProxyAtEpoch(cp string, epoch uint64) error {
	if epoch != gStartEpoch {
		return fmt.Errorf("startup canceled; not connected")
	}
	return startProxy(cp)
}

// startProxy requires gMu. Dispose the previous session before replacing globals.
func startProxy(cp string) error {
	return startProxyInDataDir(cp, resolveDataDir())
}

func startProxyInDataDir(cp, dataDir string) error {
	if gStartup != nil {
		return fmt.Errorf("startup already in progress")
	}
	if gManager != nil && gManager.IsRunning() {
		return fmt.Errorf("already running")
	}
	stopSubscriptionUpdater()
	if gHealthChecker != nil {
		gHealthChecker.Stop()
	}
	if gManager != nil {
		if err := gManager.Stop(); err != nil {
			return fmt.Errorf("stop stale sing-box lifecycle: %w", err)
		}
	}
	gManager, gHealthChecker, gDNSFlusher, gConfig = nil, nil, nil, nil

	found, err := resolveConfig(cp)
	if err != nil {
		// Fresh install or config deleted — copy the bundled placeholder.
		placeholder := filepath.Join(resourceDir(exeDir()), "config-placeholder.json")
		target := filepath.Join(dataDir, "config.json")
		if src, e := os.ReadFile(placeholder); e == nil {
			if e := os.MkdirAll(dataDir, 0755); e != nil {
				return fmt.Errorf("prepare data directory: %w", e)
			}
			if e := os.WriteFile(target, src, 0644); e == nil {
				found = target
				err = nil
			}
		}
		if err != nil {
			return fmt.Errorf("config not found and no placeholder available")
		}
	}

	gConfigPath = found
	cfg, err := config.Load(found)
	if err != nil {
		return err
	}

	// Init daily application logger (64 MiB per file, current + 3 volumes).
	if gLogger == nil {
		gLogger, _ = logger.New(filepath.Join(dataDir, "logs", "oneproxy.log"), 0, logger.DefaultBackups)
	}

	if cfg.SubscriptionURL != "" {
		ctx, cancel := context.WithCancel(context.Background())
		session := &startupSession{cancel: cancel}
		gStartup = session
		gConfig = cfg
		subscriptionUpdating = true
		gMu.Unlock()
		next, updateErr := config.PrepareStartupSubscription(ctx, cfg)
		gMu.Lock()
		cancel()
		if gStartup != session {
			return fmt.Errorf("startup canceled; not connected")
		}
		gStartup = nil
		subscriptionUpdating = false
		if updateErr == nil {
			updateErr = next.Save(found)
		}
		if updateErr != nil {
			setSubscriptionUpdateError()
			subscriptionLastErr = config.StartupSubscriptionError
			return fmt.Errorf("%s", subscriptionLastErr)
		}
		logSubscriptionUpdate(cfg, next)
		cfg = next
		setSubscriptionUpdateSuccess()
	}

	if routeMode := loadRouteModeOverride(); routeMode != "" {
		cfg.RouteMode = routeMode
	}

	gConfig = cfg

	if gLogger != nil {
		gLogger.Info("OneProxy v%s starting, route=%s, proxies=%d, port=%d", appVersion,
			cfg.RouteMode, len(cfg.GetEnabledProxies()), cfg.Unified.Port)
	}

	genCfg := filepath.Join(dataDir, "singbox_generated.json")
	ed := exeDir()

	gen := config.NewSingBoxGenerator(cfg, resourceDir(ed))
	if err := gen.SaveToFile(genCfg); err != nil {
		return err
	}

	// Find sing-box in the development layout or the installed app bundle.
	cwd, _ := filepath.Abs(".")

	for _, dir := range []string{cwd, ed} {
		ab := singBoxPath(dir)
		ab, _ = filepath.Abs(ab)
		if _, err := os.Stat(ab); err == nil {
			manager := proxy.NewManagerWithLog(ab, genCfg, filepath.Join(dataDir, "logs", "singbox.log"))
			manager.SetLogger(gLogger)
			gManager = manager
			goto started
		}
	}
	return fmt.Errorf("%s not found (cwd=%s, exe=%s)", singBoxExecutableName(), cwd, ed)

started:
	gHealthChecker = proxy.NewHealthChecker(cfg, gManager)
	gHealthChecker.SetLogger(gLogger)
	gDNSFlusher = proxy.NewDNSFlusher()
	gDNSFlusher.SetLogger(gLogger)

	// Preserve the configured manual cooldown, but all-down is not DNS evidence.
	if cfg.DNS.FlushOnFailure {
		cooldown := time.Duration(cfg.DNS.FlushIntervalSeconds) * time.Second
		if cooldown <= 0 {
			cooldown = 300 * time.Second
		}
		gDNSFlusher.SetCooldown(cooldown)
		if gLogger != nil {
			gLogger.Info("automatic DNS flush disabled: all-down checks do not establish a local DNS cache failure")
		}
	}

	if err := gManager.Start(); err != nil {
		return err
	}
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
	watchManagerLifecycle(gManager, gHealthChecker)
	startSubscriptionUpdater()
	return nil
}

// watchManagerLifecycle stops health checks when sing-box exits unexpectedly.
func watchManagerLifecycle(manager *proxy.Manager, checker *proxy.HealthChecker) {
	go func() {
		err, ok := <-manager.Done()
		if !ok {
			return
		}
		gMu.Lock()
		defer gMu.Unlock()
		if gManager != manager || gHealthChecker != checker {
			return
		}
		if checker != nil {
			checker.Stop()
		}
		stopSubscriptionUpdater()
		if gLogger != nil && err != nil {
			gLogger.Error("sing-box lifecycle ended: %v", err)
		}
		// Publish a fully stopped state so a later Start cannot retain stale
		// health/config objects from the failed lifecycle.
		gManager, gHealthChecker, gDNSFlusher, gConfig = nil, nil, nil, nil
	}()
}

//export OneProxy_Stop
func OneProxy_Stop() *C.char {
	gMu.Lock()
	defer gMu.Unlock()
	return errStr(stopProxy())
}

// Requires gMu, including when called by Restart.
func stopProxy() error {
	gStartEpoch++
	if gStartup != nil {
		gStartup.cancel()
		gStartup = nil
	}
	stopSubscriptionUpdater()
	if gLogger != nil {
		gLogger.Info("stopping")
	}
	if gHealthChecker != nil {
		gHealthChecker.Stop()
	}
	var stopErr error
	if gManager != nil {
		stopErr = gManager.Stop()
	}
	if stopErr != nil {
		return stopErr
	}
	gManager, gHealthChecker, gDNSFlusher, gConfig = nil, nil, nil, nil
	if gLogger != nil {
		gLogger.Info("stopped")
	}
	return nil
}

//export OneProxy_Restart
func OneProxy_Restart() *C.char {
	gMu.Lock()
	defer gMu.Unlock()
	if gStartup != nil {
		return errStr(fmt.Errorf("startup already in progress"))
	}
	if gManager == nil {
		return errStr(fmt.Errorf("not started"))
	}
	cp := gConfigPath
	if err := stopProxy(); err != nil {
		return errStr(err)
	}
	return errStr(startProxy(cp))
}

type statusOut struct {
	Running                bool          `json:"running"`
	UnifiedPort            int           `json:"unified_port"`
	SelectionMode          string        `json:"selection_mode,omitempty"`
	SelectedProxy          string        `json:"selected_proxy,omitempty"`
	SubscriptionConfigured bool          `json:"subscription_configured"`
	SubscriptionUpdating   bool          `json:"subscription_updating"`
	SubscriptionLastAt     string        `json:"subscription_last_at,omitempty"`
	SubscriptionLastError  string        `json:"subscription_last_error,omitempty"`
	SubscriptionResultID   string        `json:"subscription_result_id,omitempty"`
	Proxies                []statusProxy `json:"proxies"`
}

type statusProxy struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	Type       string `json:"type"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Enabled    bool   `json:"enabled"`
	IsHealthy  bool   `json:"is_healthy"`
	LatencyMS  int64  `json:"latency_ms"`
}

//export OneProxy_Status
func OneProxy_Status() *C.char {
	gMu.Lock()
	manager := gManager
	configSnapshot := gConfig
	healthChecker := gHealthChecker
	updating, lastAt, lastError, resultID := subscriptionStatus()
	gMu.Unlock()

	out := statusOut{}
	if manager != nil {
		out.Running = manager.IsRunning()
	}
	if configSnapshot != nil {
		out.UnifiedPort = configSnapshot.Unified.Port
		out.SubscriptionConfigured = configSnapshot.SubscriptionURL != ""
		if out.SubscriptionConfigured {
			out.SubscriptionUpdating = updating
			out.SubscriptionLastError = lastError
			out.SubscriptionResultID = resultID
			if !lastAt.IsZero() {
				out.SubscriptionLastAt = lastAt.Format(time.RFC3339)
			}
		}
		if out.Running && out.UnifiedPort > 0 {
			out.SelectionMode, out.SelectedProxy = selectorState(configSnapshot)
		}
		for _, p := range configSnapshot.Proxies {
			px := statusProxy{Name: p.Name, Port: p.LocalPort, Type: p.Type, Server: p.Server, ServerPort: p.Port, Enabled: p.Enabled}
			if healthChecker != nil {
				if r := healthChecker.GetResult(p.Name); r != nil {
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
	gMu.Lock()
	defer gMu.Unlock()
	if gHealthChecker == nil || gManager == nil || !gManager.IsRunning() {
		return errStr(fmt.Errorf("not running"))
	}
	gHealthChecker.CheckAll()
	return nil
}

//export OneProxy_FlushDNS
func OneProxy_FlushDNS() *C.char {
	return errStr(flushDNS((*proxy.DNSFlusher).FlushAll))
}

// flushDNS serializes the entire manual operation with Start, Stop and Restart.
func flushDNS(flush func(*proxy.DNSFlusher, *proxy.Manager) error) error {
	gMu.Lock()
	defer gMu.Unlock()
	if gDNSFlusher == nil || gManager == nil {
		return fmt.Errorf("not running")
	}
	return flush(gDNSFlusher, gManager)
}

// reloadAndRestart regenerates singbox_generated.json from gConfig and restarts
// sing-box. If sing-box is not currently running, it leaves the generated config
// in place for the next Start.
func reloadAndRestart() error {
	if gConfig == nil {
		return nil
	}
	ed := exeDir()
	genCfg := filepath.Join(resolveDataDir(), "singbox_generated.json")
	gen := config.NewSingBoxGenerator(gConfig, resourceDir(ed))
	if err := gen.SaveToFile(genCfg); err != nil {
		return err
	}
	if gHealthChecker != nil {
		gHealthChecker.Stop()
		gHealthChecker.SetConfig(gConfig)
	}
	if gManager == nil || !gManager.IsRunning() {
		if gLogger != nil {
			gLogger.Info("config saved — a restart is needed to apply")
		}
		return nil
	}
	gManager.SetConfigPath(genCfg)
	if err := gManager.Restart(); err != nil {
		return err
	}
	if gConfig.HealthCheck.Enabled && gHealthChecker != nil {
		gHealthChecker.Start()
	}
	return nil
}

//export OneProxy_ImportSubscription
func OneProxy_ImportSubscription(input *C.char) *C.char {
	raw := C.GoString(input)

	switch {
	case strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://"):
		return errStr(updateSubscription(raw, true))
	case strings.HasPrefix(raw, "ss://"), strings.HasPrefix(raw, "vmess://"), strings.HasPrefix(raw, "vless://"):
		proxy, err := config.ParseSubscriptionLine(raw)
		if err != nil {
			return errStr(fmt.Errorf("invalid proxy URL: %w", err))
		}
		return errStr(importManualProxy(proxy))
	default:
		return errStr(fmt.Errorf("not a valid subscription URL"))
	}
}

func importManualProxy(proxyConfig config.ProxyConfig) error {
	gMu.Lock()
	current, session := gConfig, subscriptionActive
	if gStartup != nil {
		gMu.Unlock()
		return fmt.Errorf("startup in progress; try again after startup")
	}
	gMu.Unlock()
	subscriptionUpdateMu.Lock()
	defer subscriptionUpdateMu.Unlock()
	gMu.Lock()
	defer gMu.Unlock()
	if gConfig == nil {
		return fmt.Errorf("not started")
	}
	if gStartup != nil {
		return fmt.Errorf("startup in progress; try again after startup")
	}
	if gConfig != current || subscriptionActive != session {
		return fmt.Errorf("subscription session changed while importing")
	}
	next, err := config.MergeManualProxy(current, proxyConfig)
	if err != nil {
		return err
	}
	configPath := filepath.Join(resolveDataDir(), "config.json")
	if err := next.Save(configPath); err != nil {
		return err
	}
	gConfig = next
	if err := reloadAndRestart(); err != nil {
		gConfig = current
		_ = current.Save(configPath)
		_ = reloadAndRestart()
		return err
	}
	return nil
}

//export OneProxy_UpdateSubscription
func OneProxy_UpdateSubscription() *C.char {
	return errStr(updateConfiguredSubscription())
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
	if selectorTag == "" {
		selectorTag = "proxy"
	}

	outboundTag, err := selectionOutboundTag(C.GoString(proxyName), cfg.Proxies)
	if err != nil {
		return errStr(err)
	}
	if err := putSelectorSelection(selectorHTTPClient, clashAPIBaseURL, selectorTag, outboundTag); err != nil {
		return errStr(err)
	}
	return nil
}

//export OneProxy_GetVersion
func OneProxy_GetVersion() *C.char { return C.CString(appVersion) }

//export OneProxy_FreeString
func OneProxy_FreeString(s *C.char) { C.free(unsafe.Pointer(s)) }

func main() {}
