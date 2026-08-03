package config

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testVLESSURI = "vless://00000000-0000-4000-8000-000000000001@edge.example.com:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=cover.example.com&fp=chrome&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=0123456789abcdef&type=tcp#JMS-1%40display.example.com%3A443"

func TestParseVLESSReality(t *testing.T) {
	px, err := ParseSubscriptionLine(testVLESSURI)
	if err != nil {
		t.Fatalf("ParseSubscriptionLine() error = %v", err)
	}

	if px.Type != "vless" {
		t.Fatalf("Type = %q, want vless", px.Type)
	}
	if px.Server != "edge.example.com" {
		t.Errorf("Server = %q, want URI host edge.example.com", px.Server)
	}
	if px.Name != "display.example.com" {
		t.Errorf("Name = %q, want display.example.com", px.Name)
	}
	if px.Port != 443 || px.UUID != "00000000-0000-4000-8000-000000000001" {
		t.Errorf("endpoint credentials were not parsed correctly: %+v", px)
	}
	if px.Security != "reality" || px.Flow != "xtls-rprx-vision" {
		t.Errorf("Security/Flow = %q/%q", px.Security, px.Flow)
	}
	if px.ServerName != "cover.example.com" || px.Fingerprint != "chrome" {
		t.Errorf("TLS fields = %q/%q", px.ServerName, px.Fingerprint)
	}
	if px.RealityPublicKey == "" || px.RealityShortID != "0123456789abcdef" {
		t.Errorf("Reality fields were not parsed correctly")
	}
}

func TestParseVLESSRejectsUnsupportedVariant(t *testing.T) {
	uri := "vless://00000000-0000-4000-8000-000000000001@edge.example.com:443?encryption=none&security=tls&type=ws"
	if _, err := ParseSubscriptionLine(uri); err == nil {
		t.Fatal("ParseSubscriptionLine() accepted unsupported VLESS variant")
	}
}

func TestFetchSubscriptionIncludesVLESS(t *testing.T) {
	ssURI := "ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@ss.example.com:8388#ss-node"
	body := base64.StdEncoding.EncodeToString([]byte(ssURI + "\n" + testVLESSURI + "\n"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	proxies, _, err := FetchSubscription(server.URL, 10801)
	if err != nil {
		t.Fatalf("FetchSubscription() error = %v", err)
	}
	if len(proxies) != 2 {
		t.Fatalf("len(proxies) = %d, want 2", len(proxies))
	}
	if proxies[0].Type != "shadowsocks" || proxies[1].Type != "vless" {
		t.Fatalf("unexpected proxy types: %q, %q", proxies[0].Type, proxies[1].Type)
	}
	if proxies[1].LocalPort != 10802 || !proxies[1].Enabled {
		t.Errorf("unexpected VLESS proxy: %+v", proxies[1])
	}
}
