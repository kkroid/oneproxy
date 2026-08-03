package config

import (
	"strings"
	"testing"
)

func TestValidateRejectsUnsupportedProxyType(t *testing.T) {
	cfg := &Config{
		Version: "1.0",
		Proxies: []ProxyConfig{{
			Name:    "unsupported-node",
			Type:    "trojan",
			Server:  "example.com",
			Port:    443,
			Enabled: true,
		}},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("Validate() error = %v, want unsupported type", err)
	}
}

func TestValidateAcceptsVLESSReality(t *testing.T) {
	cfg := &Config{
		Version: "1.0",
		Proxies: []ProxyConfig{{
			Name:             "reality-node",
			Type:             "vless",
			Server:           "edge.example.com",
			Port:             443,
			UUID:             "00000000-0000-4000-8000-000000000001",
			Security:         "reality",
			Flow:             "xtls-rprx-vision",
			ServerName:       "www.example.com",
			Fingerprint:      "chrome",
			RealityPublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			RealityShortID:   "0123456789abcdef",
		}},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
