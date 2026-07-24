package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
	"github.com/kkroid/oneproxy/internal/proxy"
)

const (
	configFile        = "config.json"
	exampleConfigFile = "configs/config.example.json"
	singboxConfigFile = "singbox_generated.json"
	singboxBinary     = "bin/sing-box.exe"
)

func main() {
	fmt.Println("OneProxy starting...")

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := config.NewSingBoxGenerator(cfg, ".").SaveToFile(singboxConfigFile); err != nil {
		log.Fatalf("Failed to generate sing-box config: %v", err)
	}
	fmt.Printf("Generated sing-box config: %s\n", singboxConfigFile)

	if _, err := os.Stat(singboxBinary); os.IsNotExist(err) {
		log.Fatalf("sing-box not found at %s", singboxBinary)
	}

	absConfigPath, _ := filepath.Abs(singboxConfigFile)
	absSingBoxPath, _ := filepath.Abs(singboxBinary)
	manager := proxy.NewManager(absSingBoxPath, absConfigPath)
	healthChecker := proxy.NewHealthChecker(cfg, manager)
	dnsFlusher := proxy.NewDNSFlusher()

	if cfg.DNS.FlushOnFailure {
		cooldown := time.Duration(cfg.DNS.FlushIntervalSeconds) * time.Second
		if cooldown <= 0 { cooldown = 300 * time.Second }
		dnsFlusher.SetCooldown(cooldown)
		healthChecker.SetAllDownCallback(func() {
			fmt.Printf("[%s] all nodes down, flushing DNS (cooldown=%vs)...", time.Now().Format("15:04:05"), int(cooldown.Seconds()))
			if err := dnsFlusher.FlushAll(manager); err != nil {
				fmt.Printf(" FAILED: %v\n", err)
			} else {
				fmt.Println(" OK")
			}
		})
	}

	fmt.Println("Starting proxy...")
	if err := manager.Start(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}
	fmt.Println("All proxies started")

	if cfg.HealthCheck.Enabled {
		healthChecker.Start()
		fmt.Printf("Health check enabled (every %ds)\n", cfg.HealthCheck.IntervalSeconds)
	}

	fmt.Println("\nProxy ports:")
	if cfg.Unified.Port > 0 {
		fmt.Printf("  127.0.0.1:%-5d -> unified (auto-select lowest latency)\n", cfg.Unified.Port)
	}
	for _, p := range cfg.GetEnabledProxies() {
		if p.LocalPort > 0 {
			fmt.Printf("  127.0.0.1:%-5d -> %s (%s)\n", p.LocalPort, p.Name, p.Type)
		}
	}
	fmt.Println("\nOneProxy running. Ctrl+C to stop.")

	// periodic status
	go func() {
		t := time.NewTicker(time.Duration(cfg.HealthCheck.IntervalSeconds) * time.Second)
		defer t.Stop()
		time.Sleep(time.Duration(cfg.HealthCheck.IntervalSeconds+5) * time.Second)
		printStatus(healthChecker)
		for range t.C {
			printStatus(healthChecker)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	healthChecker.Stop()
	manager.Stop()
	fmt.Println("OneProxy stopped")
}

func printStatus(hc *proxy.HealthChecker) {
	fmt.Printf("\n[%s] --- Health ---\n", time.Now().Format("15:04:05"))
	for name, r := range hc.GetAllResults() {
		if r.IsHealthy {
			fmt.Printf("  [OK] %-16s :%d  %dms\n", name, r.LocalPort, r.Latency.Milliseconds())
		} else {
			fmt.Printf("  [!!] %-16s :%d  %s\n", name, r.LocalPort, r.LastError)
		}
	}
}

func loadConfig() (*config.Config, error) {
	if _, err := os.Stat(configFile); err == nil {
		return config.Load(configFile)
	}
	if _, err := os.Stat(exampleConfigFile); err == nil {
		return config.Load(exampleConfigFile)
	}
	return nil, fmt.Errorf("no config found")
}
