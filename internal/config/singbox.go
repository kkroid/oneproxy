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
	Servers  []DNSServer `json:"servers"`
	Rules    []DNSRule   `json:"rules,omitempty"`
	Strategy string      `json:"strategy,omitempty"`
}

// DNSServer configuration
type DNSServer struct {
	Tag     string `json:"tag"`
	Address string `json:"address"`
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

// generateDNS creates DNS configuration
func (g *SingBoxGenerator) generateDNS() SingBoxDNS {
	dns := SingBoxDNS{
		Strategy: "prefer_ipv4",
	}

	// Add DNS servers
	for i, server := range g.userConfig.DNS.Servers {
		tag := fmt.Sprintf("dns-%d", i)
		dns.Servers = append(dns.Servers, DNSServer{
			Tag:     tag,
			Address: server,
			Detour:  "direct",
		})
	}

	// Add rule for proxy domains to use first DNS server
	if len(dns.Servers) > 0 {
		dns.Rules = []DNSRule{
			{
				DomainSuffix: []string{".portablesubmari"},
				Server:       dns.Servers[0].Tag,
			},
		}
	}

	return dns
}

// generateInbounds creates inbound configurations
// Each enabled proxy gets its own inbound port
func (g *SingBoxGenerator) generateInbounds() []SingBoxInbound {
	var inbounds []SingBoxInbound

	// Determine inbound type from config
	inboundType := g.userConfig.Inbound.ProxyType
	if inboundType == "" {
		inboundType = "socks" // default to socks5
	}

	// Create one inbound for each enabled proxy
	enabledProxies := g.userConfig.GetEnabledProxies()
	for _, proxy := range enabledProxies {
		inboundTag := fmt.Sprintf("in-%d", proxy.LocalPort)

		inbounds = append(inbounds, SingBoxInbound{
			Type:       inboundType,
			Tag:        inboundTag,
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
	for _, proxy := range enabledProxies {
		outboundTag := fmt.Sprintf("out-%s", g.sanitizeTag(proxy.Name))

		switch proxy.Type {
		case "shadowsocks":
			outbounds = append(outbounds, ShadowsocksOutbound{
				Type:       "shadowsocks",
				Tag:        outboundTag,
				Server:     proxy.Server,
				ServerPort: proxy.Port,
				Method:     proxy.Method,
				Password:   proxy.Password,
			})

		case "vmess":
			outbounds = append(outbounds, VMessOutbound{
				Type:       "vmess",
				Tag:        outboundTag,
				Server:     proxy.Server,
				ServerPort: proxy.Port,
				UUID:       proxy.UUID,
				AlterID:    proxy.AlterID,
				Security:   proxy.Security,
			})
		}
	}

	// Add direct outbound
	outbounds = append(outbounds, DirectOutbound{
		Type: "direct",
		Tag:  "direct",
	})

	return outbounds
}

// generateRoute creates routing configuration
// Each inbound is bound to its corresponding outbound
func (g *SingBoxGenerator) generateRoute() RouteConfig {
	var rules []RouteRule

	// Create one-to-one binding rules for each proxy
	enabledProxies := g.userConfig.GetEnabledProxies()
	for _, proxy := range enabledProxies {
		inboundTag := fmt.Sprintf("in-%d", proxy.LocalPort)
		outboundTag := fmt.Sprintf("out-%s", g.sanitizeTag(proxy.Name))

		rules = append(rules, RouteRule{
			Inbound:  []string{inboundTag},
			Outbound: outboundTag,
		})
	}

	// DNS traffic goes direct
	rules = append(rules, RouteRule{
		Protocol: "dns",
		Outbound: "direct",
	})

	return RouteConfig{
		Rules: rules,
	}
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
