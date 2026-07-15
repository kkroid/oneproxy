package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SingBoxConfig represents the sing-box configuration structure
type SingBoxConfig struct {
	Log       LogConfig        `json:"log"`
	DNS       SingBoxDNS       `json:"dns"`
	Inbounds  []SingBoxInbound `json:"inbounds"`
	Outbounds []interface{}    `json:"outbounds"`
	Route     RouteConfig      `json:"route"`
}

// LogConfig for sing-box logging
type LogConfig struct {
	Level  string `json:"level"`
	Output string `json:"output,omitempty"`
}

// SingBoxDNS configuration
type SingBoxDNS struct {
	Servers  []interface{} `json:"servers"`
	Rules    []DNSRule     `json:"rules,omitempty"`
	Strategy string        `json:"strategy,omitempty"`
}

// DNSServerLegacy (for old format, kept for reference)
type DNSServerLegacy struct {
	Tag     string `json:"tag"`
	Address string `json:"address"`
	Detour  string `json:"detour,omitempty"`
}

// DNSServerNew format for sing-box 1.12+
type DNSServerNew struct {
	Tag     string `json:"tag"`
	Address string `json:"address"`
	Type    string `json:"type,omitempty"`
	Detour  string `json:"detour,omitempty"`
}

// DNSRule for DNS routing
type DNSRule struct {
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	Server       string   `json:"server"`
}

// SingBoxInbound configuration
type SingBoxInbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Listen     string `json:"listen"`
	ListenPort int    `json:"listen_port"`
}

// ShadowsocksOutbound configuration
type ShadowsocksOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Method     string `json:"method"`
	Password   string `json:"password"`
}

// VMessOutbound configuration
type VMessOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	UUID       string `json:"uuid"`
	AlterID    int    `json:"alter_id"`
	Security   string `json:"security"`
}

// DirectOutbound configuration
type DirectOutbound struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

// URLTestOutbound configuration — auto-select lowest latency node
type URLTestOutbound struct {
	Type      string   `json:"type"`
	Tag       string   `json:"tag"`
	Outbounds []string `json:"outbounds"`
	URL       string   `json:"url,omitempty"`
	Interval  string   `json:"interval,omitempty"`
}

// SelectorOutbound configuration — manual or auto selection
type SelectorOutbound struct {
	Type      string   `json:"type"`
	Tag       string   `json:"tag"`
	Outbounds []string `json:"outbounds"`
	Default   string   `json:"default,omitempty"`
}

// RouteConfig for routing rules
type RouteConfig struct {
	Rules []RouteRule `json:"rules"`
	Final string      `json:"final,omitempty"`
}

// RouteRule for routing
type RouteRule struct {
	Inbound  []string `json:"inbound,omitempty"`
	Protocol string   `json:"protocol,omitempty"`
	Outbound string   `json:"outbound"`
}

// SingBoxGenerator generates sing-box configuration from user config
type SingBoxGenerator struct {
	userConfig *Config
}

// NewSingBoxGenerator creates a new generator
func NewSingBoxGenerator(cfg *Config) *SingBoxGenerator {
	return &SingBoxGenerator{userConfig: cfg}
}

// Generate creates a sing-box configuration
func (g *SingBoxGenerator) Generate() (*SingBoxConfig, error) {
	sbConfig := &SingBoxConfig{
		Log: LogConfig{
			Level:  g.userConfig.LogLevel,
			Output: "logs/singbox.log",
		},
		DNS:       g.generateDNS(),
		Inbounds:  g.generateInbounds(),
		Outbounds: g.generateOutbounds(),
		Route:     g.generateRoute(),
	}

	return sbConfig, nil
}

// generateDNS creates DNS configuration for sing-box 1.12+
func (g *SingBoxGenerator) generateDNS() SingBoxDNS {
	return SingBoxDNS{
		Strategy: "ipv4_only",
		Servers: []interface{}{
			map[string]interface{}{
				"tag":     "google-dns",
				"address": "tcp://8.8.8.8",
				"detour":  "direct",
			},
		},
	}
}

// generateInbounds creates inbound configurations
// multi-port: one inbound per proxy
// unified: one inbound on the unified_port
func (g *SingBoxGenerator) generateInbounds() []SingBoxInbound {
	var inbounds []SingBoxInbound

	inboundType := "socks"
	if g.userConfig.Inbound.ProxyType == "http" {
		inboundType = "http"
	} else if g.userConfig.Inbound.ProxyType == "mixed" {
		inboundType = "mixed"
	}

	if g.userConfig.Mode == "unified" {
		inbounds = append(inbounds, SingBoxInbound{
			Type:       inboundType,
			Tag:        "in-unified",
			Listen:     g.userConfig.Inbound.Listen,
			ListenPort: g.userConfig.UnifiedPort,
		})
		return inbounds
	}

	// multi-port: one inbound for each enabled proxy
	for _, proxy := range g.userConfig.GetEnabledProxies() {
		inbounds = append(inbounds, SingBoxInbound{
			Type:       inboundType,
			Tag:        fmt.Sprintf("in-%d", proxy.LocalPort),
			Listen:     g.userConfig.Inbound.Listen,
			ListenPort: proxy.LocalPort,
		})
	}
	return inbounds
}

// generateOutbounds creates outbound configurations
func (g *SingBoxGenerator) generateOutbounds() []interface{} {
	var outbounds []interface{}

	// Add proxy outbounds (one for each enabled proxy)
	enabledProxies := g.userConfig.GetEnabledProxies()
	var proxyTags []string
	for _, proxy := range enabledProxies {
		outboundTag := fmt.Sprintf("out-%s", g.sanitizeTag(proxy.Name))
		proxyTags = append(proxyTags, outboundTag)

		switch proxy.Type {
		case "shadowsocks":
			outbounds = append(outbounds, ShadowsocksOutbound{
				Type: "shadowsocks", Tag: outboundTag,
				Server: proxy.Server, ServerPort: proxy.Port,
				Method: proxy.Method, Password: proxy.Password,
			})
		case "vmess":
			outbounds = append(outbounds, VMessOutbound{
				Type: "vmess", Tag: outboundTag,
				Server: proxy.Server, ServerPort: proxy.Port,
				UUID: proxy.UUID, AlterID: proxy.AlterID, Security: proxy.Security,
			})
		}
	}

	if g.userConfig.Mode == "unified" && len(proxyTags) > 0 {
		// URLTest → auto-select lowest latency
		outbounds = append(outbounds, URLTestOutbound{
			Type:      "urltest",
			Tag:       "auto",
			Outbounds: proxyTags,
		})
		// Selector → manual override with "auto" as default
		selectorTags := append([]string{"auto"}, proxyTags...)
		outbounds = append(outbounds, SelectorOutbound{
			Type:      "selector",
			Tag:       "proxy",
			Outbounds: selectorTags,
			Default:   "auto",
		})
	}

	// Direct
	outbounds = append(outbounds, DirectOutbound{Type: "direct", Tag: "direct"})
	return outbounds
}

// generateRoute creates routing configuration
// multi-port: each inbound → its corresponding outbound
// unified: final = "proxy" (selector), no per-inbound rules
func (g *SingBoxGenerator) generateRoute() RouteConfig {
	var rules []RouteRule

	if g.userConfig.Mode == "unified" {
		// DNS through direct, everything else through the selector
		rules = append(rules, RouteRule{Protocol: "dns", Outbound: "direct"})
		return RouteConfig{Rules: rules, Final: "proxy"}
	}

	// multi-port: one-to-one binding
	for _, proxy := range g.userConfig.GetEnabledProxies() {
		inboundTag := fmt.Sprintf("in-%d", proxy.LocalPort)
		outboundTag := fmt.Sprintf("out-%s", g.sanitizeTag(proxy.Name))
		rules = append(rules, RouteRule{Inbound: []string{inboundTag}, Outbound: outboundTag})
	}
	rules = append(rules, RouteRule{Protocol: "dns", Outbound: "direct"})
	return RouteConfig{Rules: rules}
}

// sanitizeTag creates a valid tag from proxy name
func (g *SingBoxGenerator) sanitizeTag(name string) string {
	// Simple sanitization: replace spaces and special chars
	tag := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			tag += string(c)
		} else if c == ' ' {
			tag += "-"
		}
	}
	return tag
}

// SaveToFile writes sing-box config to file
func (g *SingBoxGenerator) SaveToFile(path string) error {
	sbConfig, err := g.Generate()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(sbConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sing-box config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write sing-box config: %w", err)
	}

	return nil
}
