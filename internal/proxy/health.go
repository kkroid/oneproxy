package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/proxy"
	"github.com/kkroid/oneproxy/internal/config"
	"github.com/kkroid/oneproxy/internal/logger"
)

// HealthResult represents the health check result for a single proxy
type HealthResult struct {
	ProxyName  string
	LocalPort  int
	IsHealthy  bool
	Latency    time.Duration
	LastCheck  time.Time
	ErrorCount int
	LastError  string
}

// HealthChecker manages health checking for all proxies
type HealthChecker struct {
	config       *config.Config
	manager      *Manager
	results      map[string]*HealthResult
	resultsMux   sync.RWMutex
	stateMux     sync.Mutex // guards isRunning, stopChan, wg, allDownCount
	stopChan     chan struct{}
	isRunning    bool
	stopped      bool           // true once Stop() has closed stopChan
	wg           sync.WaitGroup // tracks in-flight CheckAll rounds
	allDownCount int            // consecutive all-down rounds
	onAllDown    func()         // callback when ALL nodes are down
	appLog       *logger.Logger
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(cfg *config.Config, manager *Manager) *HealthChecker {
	hc := &HealthChecker{
		config:   cfg,
		manager:  manager,
		results:  make(map[string]*HealthResult),
		stopChan: make(chan struct{}),
	}

	for _, proxy := range cfg.GetEnabledProxies() {
		hc.results[proxy.Name] = &HealthResult{
			ProxyName: proxy.Name,
			LocalPort: proxy.LocalPort,
		}
	}

	return hc
}

// Start begins periodic health checking with adaptive backoff.
func (hc *HealthChecker) Start() {
	hc.stateMux.Lock()
	defer hc.stateMux.Unlock()

	if hc.isRunning || !hc.config.HealthCheck.Enabled {
		return
	}

	hc.isRunning = true
	hc.stopped = false
	hc.allDownCount = 0
	hc.stopChan = make(chan struct{})
	stop := hc.stopChan

	hc.wg.Add(1)
	go func() {
		defer hc.wg.Done()
		hc.CheckAll() // first check immediately
		for {
			interval := hc.nextInterval()
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
				hc.CheckAll()
			case <-stop:
				timer.Stop()
				return
			}
		}
	}()
}

// Stop stops the health checker and waits for the current CheckAll
// round to finish. This prevents stale checks from racing with Restart.
func (hc *HealthChecker) Stop() {
	hc.stateMux.Lock()
	if !hc.isRunning {
		hc.stateMux.Unlock()
		return
	}
	hc.isRunning = false
	hc.stopped = true
	close(hc.stopChan)
	hc.stateMux.Unlock()

	// Wait for in-flight CheckAll to complete — prevents stale checks
	// from triggering all-down callbacks after sing-box restarts.
	hc.wg.Wait()
}

// nextInterval returns the adaptive check interval based on current health.
func (hc *HealthChecker) nextInterval() time.Duration {
	hc.resultsMux.RLock()
	defer hc.resultsMux.RUnlock()

	ok, total := 0, 0
	for _, r := range hc.results {
		if r.LocalPort > 0 {
			total++
			if r.IsHealthy { ok++ }
		}
	}
	if total == 0 || ok == total {
		return 60 * time.Second // all green — relax
	}
	if ok == 0 {
		return 10 * time.Second // all red — moderate
	}
	return 5 * time.Second // partial degradation — aggressive
}

// CheckAll checks all enabled proxies concurrently.
// Requires 3 consecutive all-down rounds before triggering recovery,
// and never triggers during recovery or after Stop.
func (hc *HealthChecker) CheckAll() {
	hc.wg.Add(1)
	defer hc.wg.Done()

	cfg := hc.config

	// Check all nodes concurrently
	var innerWg sync.WaitGroup
	for _, proxy := range cfg.GetEnabledProxies() {
		innerWg.Add(1)
		go func(p config.ProxyConfig) {
			defer innerWg.Done()
			hc.CheckProxy(p.Name, p.LocalPort)
		}(proxy)
	}
	innerWg.Wait()

	ok, total := 0, 0
	hc.resultsMux.RLock()
	for _, r := range hc.results {
		if r.LocalPort > 0 {
			total++
			if r.IsHealthy { ok++ }
		}
	}
	hc.resultsMux.RUnlock()

	hc.stateMux.Lock()
	defer hc.stateMux.Unlock()

	// Already stopped — don't trigger callbacks.
	if hc.stopped {
		return
	}

	if total == 0 || ok > 0 {
		// Any node healthy → reset all-down counter.
		hc.allDownCount = 0
		return
	}

	// All nodes are down.
	hc.allDownCount++
	if hc.allDownCount < 3 || hc.onAllDown == nil {
		return
	}

	// 3 consecutive all-down rounds → trigger recovery. Mark stopped
	// to prevent re-triggering until Start() resets the state.
	if hc.appLog != nil {
		hc.appLog.Warn("all %d nodes down for 3 rounds — triggering recovery", total)
	}
	hc.allDownCount = 0 // reset so if recovery fails we can fire again
	go hc.onAllDown()
}

// CheckProxy checks a single proxy's health
func (hc *HealthChecker) CheckProxy(proxyName string, localPort int) *HealthResult {
	hc.resultsMux.Lock()
	result, exists := hc.results[proxyName]
	if !exists {
		result = &HealthResult{ProxyName: proxyName, LocalPort: localPort}
		hc.results[proxyName] = result
	}
	hc.resultsMux.Unlock()

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), nil, proxy.Direct)
	if err != nil {
		hc.updateResult(proxyName, false, 0, fmt.Sprintf("dialer: %v", err))
		return result
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(hc.config.HealthCheck.TimeoutSeconds) * time.Second,
	}

	testURL := hc.config.HealthCheck.TestURL
	if testURL == "" {
		testURL = "https://www.google.com/generate_204"
	}

	startTime := time.Now()
	resp, err := client.Get(testURL)
	latency := time.Since(startTime)

	if err != nil {
		hc.updateResult(proxyName, false, latency, fmt.Sprintf("%v", err))
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		hc.updateResult(proxyName, true, latency, "")
	} else {
		hc.updateResult(proxyName, false, latency, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}

	return result
}

// updateResult updates the health check result
func (hc *HealthChecker) updateResult(proxyName string, isHealthy bool, latency time.Duration, errorMsg string) {
	hc.resultsMux.Lock()
	defer hc.resultsMux.Unlock()

	result := hc.results[proxyName]
	wasHealthy := result.IsHealthy
	result.LastCheck = time.Now()
	result.Latency = latency

	if isHealthy {
		result.IsHealthy = true
		result.ErrorCount = 0
		result.LastError = ""
		if !wasHealthy && hc.appLog != nil {
			hc.appLog.Info("node %s recovered (healthy, %v)", proxyName, latency.Round(time.Millisecond))
		}
	} else {
		result.IsHealthy = false
		result.ErrorCount++
		result.LastError = errorMsg
		if wasHealthy && hc.appLog != nil {
			hc.appLog.Warn("node %s went unhealthy: %s", proxyName, errorMsg)
		}
	}
}

// GetResult returns the health check result for a specific proxy
func (hc *HealthChecker) GetResult(proxyName string) *HealthResult {
	hc.resultsMux.RLock()
	defer hc.resultsMux.RUnlock()

	result, exists := hc.results[proxyName]
	if !exists {
		return nil
	}
	resultCopy := *result
	return &resultCopy
}

// GetAllResults returns all health check results
func (hc *HealthChecker) GetAllResults() map[string]*HealthResult {
	hc.resultsMux.RLock()
	defer hc.resultsMux.RUnlock()

	results := make(map[string]*HealthResult)
	for name, result := range hc.results {
		resultCopy := *result
		results[name] = &resultCopy
	}
	return results
}

// SetAllDownCallback sets the callback for when every node goes down.
func (hc *HealthChecker) SetAllDownCallback(callback func()) {
	hc.onAllDown = callback
}

// SetLogger sets the application logger.
func (hc *HealthChecker) SetLogger(l *logger.Logger) {
	hc.appLog = l
}

// SetConfig updates the config reference (used after import/restore).
// Rebuilds the results map, preserving state for same-named nodes.
func (hc *HealthChecker) SetConfig(cfg *config.Config) {
	hc.resultsMux.Lock()
	defer hc.resultsMux.Unlock()

	hc.config = cfg
	newResults := make(map[string]*HealthResult)
	for _, p := range cfg.GetEnabledProxies() {
		if old, ok := hc.results[p.Name]; ok {
			// Preserve existing state
			copy := *old
			newResults[p.Name] = &copy
		} else {
			newResults[p.Name] = &HealthResult{
				ProxyName: p.Name,
				LocalPort: p.LocalPort,
			}
		}
	}
	hc.results = newResults
}

// IsRunning returns whether the health checker is running
func (hc *HealthChecker) IsRunning() bool {
	hc.stateMux.Lock()
	defer hc.stateMux.Unlock()
	return hc.isRunning
}
