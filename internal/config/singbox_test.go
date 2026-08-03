package config

import (
	"encoding/json"
	"testing"
)

func TestGenerateVLESSRealityOutbound(t *testing.T) {
	cfg := &Config{
		Version:  "1.0",
		LogLevel: "error",
		Proxies: []ProxyConfig{{
			Name:             "reality-node",
			Enabled:          true,
			Type:             "vless",
			Server:           "edge.example.com",
			Port:             443,
			UUID:             "00000000-0000-4000-8000-000000000001",
			Security:         "reality",
			Flow:             "xtls-rprx-vision",
			ServerName:       "cover.example.com",
			Fingerprint:      "chrome",
			RealityPublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			RealityShortID:   "0123456789abcdef",
		}},
	}

	generated, err := NewSingBoxGenerator(cfg, t.TempDir()).Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	data, err := json.Marshal(generated.Outbounds[0])
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(data, &outbound); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if outbound["type"] != "vless" || outbound["flow"] != "xtls-rprx-vision" {
		t.Fatalf("unexpected outbound: %s", data)
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok || tls["enabled"] != true || tls["server_name"] != "cover.example.com" {
		t.Fatalf("unexpected TLS options: %v", outbound["tls"])
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("unexpected uTLS options: %v", tls["utls"])
	}
	reality, ok := tls["reality"].(map[string]interface{})
	if !ok || reality["public_key"] == "" || reality["short_id"] != "0123456789abcdef" {
		t.Fatalf("unexpected Reality options: %v", tls["reality"])
	}
}
