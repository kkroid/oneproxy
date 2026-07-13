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
)

// HealthResult represents the health check result for a single proxy
type HealthResult struct {
	ProxyName    string
	LocalPort    int
	IsHealthy    bool
	Latency      time.Duration
	LastCheck    time.Time
	ErrorCount   int
	LastError    string
}

// HealthChecker manages health checking for all proxies
type HealthChecker struct {
	config      *config.Config
	manager     *Manager
	results     map[string]*HealthResult
	resultsMux  sync.RWMutex
	ticker      *time.Ticker
	stopChan    chan struct{}
	isRunning   bool
	onFailure   func(proxyName string) // callback when proxy fails
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(cfg *config.Config, manager *Manager) *HealthChecker {
	hc := &HealthChecker{
		config:    cfg,
		manager:   manager,
		results:   make(map[string]*HealthResult),
		stopChan:  make(chan struct{}),
		isRunning: false,
	}

	// Initialize results for all enabled proxies
	for _, proxy := range cfg.GetEnabledProxies() {
		hc.results[proxy.Name] = &HealthResult{
			ProxyName:  proxy.Name,
			LocalPort:  proxy.LocalPort,
			IsHealthy:  false,
			ErrorCount: 0,
		}
	}

	return hc
}

// Start begins periodic health checking
func (hc *HealthChecker) Start() {
	if hc.isRunning {
		return
	}

	if !hc.config.HealthCheck.Enabled {
		return
	}

	hc.isRunning = true
	interval := time.Duration(hc.config.HealthCheck.IntervalSeconds) * time.Second
	hc.ticker = time.NewTicker(interval)

	// Run initial check immediately
	go hc.CheckAll()

	// Start periodic checking
	go func() {
		for {
			select {
			case <-hc.ticker.C:
				hc.CheckAll()
			case <-hc.stopChan:
				return
			}
		}
	}()
}

// Stop stops the health checker
func (hc *HealthChecker) Stop() {
	if !hc.isRunning {
		return
	}

	hc.isRunning = false
	if hc.ticker != nil {
		hc.ticker.Stop()
	}
	close(hc.stopChan)
}

// CheckAll checks all enabled proxies concurrently
func (hc *HealthChecker) CheckAll() {
	var wg sync.WaitGroup

	for _, proxy := range hc.config.GetEnabledProxies() {
		wg.Add(1)
		go func(p config.ProxyConfig) {
			defer wg.Done()
			hc.CheckProxy(p.Name, p.LocalPort)
		}(proxy)
	}

	wg.Wait()
}

// CheckProxy checks a single proxy's health
func (hc *HealthChecker) CheckProxy(proxyName string, localPort int) *HealthResult {
	hc.resultsMux.Lock()
	result, exists := hc.results[proxyName]
	if !exists {
		result = &HealthResult{
			ProxyName: proxyName,
			LocalPort: localPort,
		}
		hc.results[proxyName] = result
	}
	hc.resultsMux.Unlock()

	// Create SOCKS5 dialer
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), nil, proxy.Direct)
	if err != nil {
		hc.updateResult(proxyName, false, 0, fmt.Sprintf("Failed to create dialer: %v", err))
		return result
	}

	// Create HTTP client with SOCKS5 proxy
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(hc.config.HealthCheck.TimeoutSeconds) * time.Second,
	}

	// Measure latency
	startTime := time.Now()

	// Make request to test URL
	testURL := hc.config.HealthCheck.TestURL
	if testURL == "" {
		testURL = "https://www.google.com/generate_204"
	}

	resp, err := client.Get(testURL)
	latency := time.Since(startTime)

	if err != nil {
		hc.updateResult(proxyName, false, latency, fmt.Sprintf("Request failed: %v", err))
		return result
	}
	defer resp.Body.Close()

	// Check response status
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
	result.LastCheck = time.Now()
	result.Latency = latency

	if isHealthy {
		result.IsHealthy = true
		result.ErrorCount = 0
		result.LastError = ""
	} else {
		result.IsHealthy = false
		result.ErrorCount++
		result.LastError = errorMsg

		// Trigger failure callback if error count reaches threshold
		if result.ErrorCount >= 3 && hc.onFailure != nil {
			go hc.onFailure(proxyName)
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

	// Return a copy to avoid race conditions
	resultCopy := *result
	return &resultCopy
}

// GetAllResults returns all health check results
func (hc *HealthChecker) GetAllResults() map[string]*HealthResult {
	hc.resultsMux.RLock()
	defer hc.resultsMux.RUnlock()

	// Return a copy of all results
	results := make(map[string]*HealthResult)
	for name, result := range hc.results {
		resultCopy := *result
		results[name] = &resultCopy
	}

	return results
}

// SetFailureCallback sets the callback function for when a proxy fails
func (hc *HealthChecker) SetFailureCallback(callback func(proxyName string)) {
	hc.onFailure = callback
}

// IsRunning returns whether the health checker is running
func (hc *HealthChecker) IsRunning() bool {
	return hc.isRunning
}
