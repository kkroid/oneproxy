package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SingBoxConfig represents the sing-box configuration structure
type SingBoxConfig struct {
	Log          LogConfig          `json:"log"`
	DNS          SingBoxDNS         `json:"dns"`
	Inbounds     []SingBoxInbound   `json:"inbounds"`
	Outbounds    []interface{}      `json:"outbounds"`
	Route        RouteConfig        `json:"route"`
	Experimental ExperimentalConfig `json:"experimental,omitempty"`
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
	Tolerance uint     `json:"tolerance,omitempty"`
}

// SelectorOutbound configuration — manual or auto selection
type SelectorOutbound struct {
	Type      string   `json:"type"`
	Tag       string   `json:"tag"`
	Outbounds []string `json:"outbounds"`
	Default   string   `json:"default,omitempty"`
}

// ExperimentalConfig for sing-box experimental features
type ExperimentalConfig struct {
	ClashAPI ClashAPIConfig `json:"clash_api,omitempty"`
}

// ClashAPIConfig for Selector control
type ClashAPIConfig struct {
	ExternalController string `json:"external_controller"`
}

// RouteConfig for routing rules
type RouteConfig struct {
	Rules    []RouteRule        `json:"rules"`
	Final    string             `json:"final,omitempty"`
	RuleSet  []RuleSetEntry     `json:"rule_set,omitempty"`
}

// RuleSetEntry defines a named rule set
type RuleSetEntry struct {
	Type   string         `json:"type"`
	Tag    string         `json:"tag"`
	Format string         `json:"format,omitempty"`
	Path   string         `json:"path,omitempty"`
	Rules  []HeadlessRule `json:"rules,omitempty"`
}

// HeadlessRule matches traffic without an outbound (used inside rule-sets)
type HeadlessRule struct {
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
}

// RouteRule for routing
type RouteRule struct {
	Inbound  []string `json:"inbound,omitempty"`
	Protocol string   `json:"protocol,omitempty"`
	Outbound string   `json:"outbound"`
	RuleSet  []string `json:"rule_set,omitempty"`
}

// SingBoxGenerator generates sing-box configuration from user config
type SingBoxGenerator struct {
	userConfig *Config
	baseDir    string // absolute path to the install/exe dir (for db paths)
}

// NewSingBoxGenerator creates a new generator. baseDir is the directory
// containing the bin/ folder with geoip.db/geosite.db.
func NewSingBoxGenerator(cfg *Config, baseDir string) *SingBoxGenerator {
	return &SingBoxGenerator{userConfig: cfg, baseDir: baseDir}
}

// Generate creates a sing-box configuration
func (g *SingBoxGenerator) Generate() (*SingBoxConfig, error) {
	sbConfig := &SingBoxConfig{
		Log: LogConfig{
			Level: g.userConfig.LogLevel,
		},
		DNS:       g.generateDNS(),
		Inbounds:  g.generateInbounds(),
		Outbounds: g.generateOutbounds(),
		Route:     g.generateRoute(),
	}

	// Clash API for Selector control (only when unified port is enabled)
	if g.userConfig.Unified.Port > 0 {
		sbConfig.Experimental = ExperimentalConfig{
			ClashAPI: ClashAPIConfig{ExternalController: "127.0.0.1:9090"},
		}
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

func (g *SingBoxGenerator) generateInbounds() []SingBoxInbound {
	var inbounds []SingBoxInbound

	inboundType := "socks"
	if g.userConfig.Inbound.ProxyType == "http" {
		inboundType = "http"
	} else if g.userConfig.Inbound.ProxyType == "mixed" {
		inboundType = "mixed"
	}

	// Individual ports — one per proxy that has local_port set
	for _, proxy := range g.userConfig.GetEnabledProxies() {
		if proxy.LocalPort <= 0 {
			continue
		}
		inbounds = append(inbounds, SingBoxInbound{
			Type:       inboundType,
			Tag:        fmt.Sprintf("in-%d", proxy.LocalPort),
			Listen:     g.userConfig.Inbound.Listen,
			ListenPort: proxy.LocalPort,
		})
	}

	// Unified port — auto-select + manual override
	if g.userConfig.Unified.Port > 0 {
		inbounds = append(inbounds, SingBoxInbound{
			Type:       inboundType,
			Tag:        "in-unified",
			Listen:     g.userConfig.Inbound.Listen,
			ListenPort: g.userConfig.Unified.Port,
		})
	}

	return inbounds
}

// generateOutbounds creates outbound configurations
func (g *SingBoxGenerator) generateOutbounds() []interface{} {
	var outbounds []interface{}

	enabledProxies := g.userConfig.GetEnabledProxies()
	var proxyTags []string
	for _, proxy := range enabledProxies {
		tag := fmt.Sprintf("out-%s", g.sanitizeTag(proxy.Name))
		proxyTags = append(proxyTags, tag)

		switch proxy.Type {
		case "shadowsocks":
			outbounds = append(outbounds, ShadowsocksOutbound{
				Type: "shadowsocks", Tag: tag,
				Server: proxy.Server, ServerPort: proxy.Port,
				Method: proxy.Method, Password: proxy.Password,
			})
		case "vmess":
			outbounds = append(outbounds, VMessOutbound{
				Type: "vmess", Tag: tag,
				Server: proxy.Server, ServerPort: proxy.Port,
				UUID: proxy.UUID, AlterID: proxy.AlterID, Security: proxy.Security,
			})
		}
	}

	// Unified selector — always present if unified port is set
	if g.userConfig.Unified.Port > 0 {
		selectorTag := "proxy"
		if g.userConfig.Unified.Tag != "" {
			selectorTag = g.userConfig.Unified.Tag
		}

		// urltest → auto-select lowest latency (tolerance=100ms prevents ping-pong)
		outbounds = append(outbounds, URLTestOutbound{
			Type: "urltest", Tag: "auto",
			Outbounds: proxyTags,
			Tolerance: 100,
		})
		// selector → manual override, default = "auto"
		selTags := append([]string{"auto"}, proxyTags...)
		outbounds = append(outbounds, SelectorOutbound{
			Type: "selector", Tag: selectorTag,
			Outbounds: selTags, Default: "auto",
		})
	}

	outbounds = append(outbounds, DirectOutbound{Type: "direct", Tag: "direct"})
	return outbounds
}

func (g *SingBoxGenerator) generateRoute() RouteConfig {
	var rules []RouteRule

	// Individual port bindings
	for _, proxy := range g.userConfig.GetEnabledProxies() {
		if proxy.LocalPort <= 0 {
			continue
		}
		inTag := fmt.Sprintf("in-%d", proxy.LocalPort)
		outTag := fmt.Sprintf("out-%s", g.sanitizeTag(proxy.Name))
		rules = append(rules, RouteRule{Inbound: []string{inTag}, Outbound: outTag})
	}

	// Unified port → selector
	if g.userConfig.Unified.Port > 0 {
		selectorTag := "proxy"
		if g.userConfig.Unified.Tag != "" {
			selectorTag = g.userConfig.Unified.Tag
		}
		rules = append(rules, RouteRule{Inbound: []string{"in-unified"}, Outbound: selectorTag})
	}

	rules = append(rules, RouteRule{Protocol: "dns", Outbound: "direct"})

	// Routing mode — global / rule / direct
	rc := RouteConfig{Rules: rules}
	switch g.userConfig.RouteMode {
	case "direct":
		rc.Final = "direct"
	case "rule":
		rc.Final = "proxy"
		rc.RuleSet = []RuleSetEntry{
			{Type: "local", Tag: "geoip-cn",  Format: "source", Path: filepath.Join(g.baseDir, "bin", "geoip-cn.json")},
			{Type: "local", Tag: "geosite-cn", Format: "source", Path: filepath.Join(g.baseDir, "bin", "geosite-cn.json")},
		}
		rc.Rules = append(rc.Rules,
			RouteRule{RuleSet: []string{"geoip-cn"},  Outbound: "direct"},
			RouteRule{RuleSet: []string{"geosite-cn"}, Outbound: "direct"})
	default: // "global" or empty
		rc.Final = "proxy"
	}
	return rc
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
