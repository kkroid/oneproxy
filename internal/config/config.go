package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the main configuration structure
type Config struct {
	Version     string        `json:"version"`
	LogLevel    string        `json:"log_level"`
	Mode        string        `json:"mode,omitempty"`        // "multi-port" (default) or "unified"
	UnifiedPort int           `json:"unified_port,omitempty"` // single port for unified mode
	HealthCheck HealthCheck   `json:"health_check"`
	DNS         DNSConfig     `json:"dns"`
	Proxies     []ProxyConfig `json:"proxies"`
	Inbound     InboundConfig `json:"inbound"`
}

// HealthCheck configuration
type HealthCheck struct {
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	TestURL         string `json:"test_url"`
}

// DNSConfig configuration
type DNSConfig struct {
	FlushOnFailure        bool     `json:"flush_on_failure"`
	FlushIntervalSeconds  int      `json:"flush_interval_seconds"`
	Servers               []string `json:"servers"`
}

// ProxyConfig represents a single proxy server configuration
type ProxyConfig struct {
	Name      string                 `json:"name"`
	Enabled   bool                   `json:"enabled"`
	LocalPort int                    `json:"local_port"`         // local port to expose this proxy
	Type      string                 `json:"type"`               // shadowsocks, vmess, trojan, etc.
	Server    string                 `json:"server"`
	Port      int                    `json:"port"`
	Method    string                 `json:"method,omitempty"`   // for shadowsocks
	Password  string                 `json:"password,omitempty"` // for shadowsocks, trojan
	UUID      string                 `json:"uuid,omitempty"`     // for vmess
	AlterID   int                    `json:"alter_id,omitempty"` // for vmess
	Security  string                 `json:"security,omitempty"` // for vmess
	Extra     map[string]interface{} `json:"extra,omitempty"`    // for additional fields
}

// InboundConfig represents local listening configuration
type InboundConfig struct {
	Listen    string `json:"listen"`
	ProxyType string `json:"proxy_type"` // socks5, http, or mixed
}

// Load reads configuration from file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Save writes configuration to file
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}

	if len(c.Proxies) == 0 {
		return fmt.Errorf("at least one proxy is required")
	}

	if c.Mode == "" {
		c.Mode = "multi-port" // default
	}

	// Validate each proxy
	portMap := make(map[int]string) // track local port usage (multi-port mode only)
	for i, proxy := range c.Proxies {
		if proxy.Name == "" {
			return fmt.Errorf("proxy #%d: name is required", i)
		}
		if proxy.Type == "" {
			return fmt.Errorf("proxy %s: type is required", proxy.Name)
		}
		if proxy.Server == "" {
			return fmt.Errorf("proxy %s: server is required", proxy.Name)
		}
		if proxy.Port <= 0 || proxy.Port > 65535 {
			return fmt.Errorf("proxy %s: invalid port %d", proxy.Name, proxy.Port)
		}

		// local_port validation only for multi-port mode
		if c.Mode == "multi-port" {
			if proxy.LocalPort <= 0 || proxy.LocalPort > 65535 {
				return fmt.Errorf("proxy %s: invalid local_port %d", proxy.Name, proxy.LocalPort)
			}
			if existingProxy, exists := portMap[proxy.LocalPort]; exists {
				return fmt.Errorf("proxy %s: local_port %d conflicts with proxy %s", proxy.Name, proxy.LocalPort, existingProxy)
			}
			portMap[proxy.LocalPort] = proxy.Name
		}

		// Type-specific validation
		switch proxy.Type {
		case "shadowsocks":
			if proxy.Method == "" {
				return fmt.Errorf("proxy %s: method is required for shadowsocks", proxy.Name)
			}
			if proxy.Password == "" {
				return fmt.Errorf("proxy %s: password is required for shadowsocks", proxy.Name)
			}
		case "vmess":
			if proxy.UUID == "" {
				return fmt.Errorf("proxy %s: uuid is required for vmess", proxy.Name)
			}
		}
	}

	// Validate inbound
	if c.Inbound.Listen == "" {
		return fmt.Errorf("inbound listen address is required")
	}
	if c.Inbound.ProxyType == "" {
		c.Inbound.ProxyType = "socks5" // default
	}
	if c.Inbound.ProxyType != "socks5" && c.Inbound.ProxyType != "http" && c.Inbound.ProxyType != "mixed" {
		return fmt.Errorf("inbound proxy_type must be socks5, http, or mixed")
	}

	// unified mode: unified_port is required
	if c.Mode == "unified" {
		if c.UnifiedPort <= 0 || c.UnifiedPort > 65535 {
			return fmt.Errorf("unified_port is required in unified mode")
		}
	}

	return nil
}

// GetEnabledProxies returns only enabled proxies
func (c *Config) GetEnabledProxies() []ProxyConfig {
	var enabled []ProxyConfig
	for _, proxy := range c.Proxies {
		if proxy.Enabled {
			enabled = append(enabled, proxy)
		}
	}
	return enabled
}
