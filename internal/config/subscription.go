package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// FetchSubscription downloads and parses a subscription URL (JMS/v2ray format).
// Returns full list of proxies with auto-assigned names and local ports.
func FetchSubscription(subURL string, startPort int) ([]ProxyConfig, string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(subURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("subscription server returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, "", fmt.Errorf("read subscription body: %w", err)
	}

	raw := strings.TrimSpace(string(body))
	decoded, err := decodeSubscriptionBase64(raw)
	if err != nil {
		return nil, "", fmt.Errorf("subscription is not valid base64: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	var proxies []ProxyConfig
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		px, err := parseSubscriptionLine(line)
		if err != nil {
			continue // skip unrecognized protocols (vless, trojan, etc.)
		}
		proxies = append(proxies, px)
	}

	for i := range proxies {
		if proxies[i].Name == "" {
			proxies[i].Name = fmt.Sprintf("Server%d", i+1)
		}
		proxies[i].LocalPort = startPort + i
		proxies[i].Enabled = true
	}

	return proxies, subURL, nil
}

func parseSubscriptionLine(line string) (ProxyConfig, error) {
	switch {
	case strings.HasPrefix(line, "ss://"):
		return parseShadowsocks(line)
	case strings.HasPrefix(line, "vmess://"):
		return parseVMess(line)
	default:
		return ProxyConfig{}, fmt.Errorf("unsupported protocol: %s", line[:min(20, len(line))])
	}
}

// ── Fragment & name extraction ────────────────────────────────────────
// fragment format: "JMS-746476@c702s1.portablesubmarines.com:15699"
// Returns the name ("Server-1") and the hostname ("c702s1.portablesubmarines.com").
func extractName(fragment string) (name, hostname string) {
	if idx := strings.LastIndex(fragment, "@"); idx >= 0 {
		hostPart := fragment[idx+1:]
		if ci := strings.Index(hostPart, ":"); ci >= 0 {
			hostname = hostPart[:ci]
		} else {
			hostname = hostPart
		}
		if dots := strings.Split(hostname, "."); len(dots) > 0 {
			sub := dots[0] // "c702s1"
			if si := strings.LastIndex(sub, "s"); si >= 0 && si+1 < len(sub) {
				name = "Server-" + sub[si+1:]
				return
			}
		}
	}
	name = fragment
	return
}

// ─── Shadowsocks ──────────────────────────────────────────────────
func parseShadowsocks(uri string) (ProxyConfig, error) {
	raw := strings.TrimPrefix(uri, "ss://")
	var px ProxyConfig
	px.Type = "shadowsocks"

	fragment := ""
	if idx := strings.LastIndex(raw, "#"); idx >= 0 {
		fragment, _ = url.QueryUnescape(raw[idx+1:])
		raw = raw[:idx]
	}

	if idx := strings.Index(raw, "@"); idx >= 0 {
		userinfo := raw[:idx]
		serverPart := raw[idx+1:]
		dec, err := decodeSubscriptionBase64(userinfo)
		if err != nil {
			return px, fmt.Errorf("decode ss userinfo: %w", err)
		}
		if ci := strings.Index(dec, ":"); ci >= 0 {
			px.Method = dec[:ci]
			px.Password = dec[ci+1:]
		}
		parts := strings.Split(serverPart, ":")
		if len(parts) < 2 {
			return px, fmt.Errorf("invalid ss server:port")
		}
		px.Server = parts[0]
		px.Port, _ = strconv.Atoi(parts[1])
	} else {
		dec, err := decodeSubscriptionBase64(raw)
		if err != nil {
			return px, fmt.Errorf("decode ss: %w", err)
		}
		atIdx := strings.LastIndex(dec, "@")
		if atIdx < 0 {
			return px, fmt.Errorf("invalid ss format")
		}
		if ci := strings.Index(dec[:atIdx], ":"); ci >= 0 {
			px.Method = dec[:atIdx][:ci]
			px.Password = dec[:atIdx][ci+1:]
		}
		parts := strings.Split(dec[atIdx+1:], ":")
		if len(parts) < 2 {
			return px, fmt.Errorf("invalid ss server:port")
		}
		px.Server = parts[0]
		px.Port, _ = strconv.Atoi(parts[1])
	}

	if fragment != "" {
		name, hostname := extractName(fragment)
		px.Name = name
		if hostname != "" {
			px.Server = hostname
		}
	}
	return px, nil
}

// ─── VMess ────────────────────────────────────────────────────────
type vmessJSON struct {
	PS   string `json:"ps"`
	Port string `json:"port"`
	ID   string `json:"id"`
	AID  int    `json:"aid"`
	Net  string `json:"net"`
	Type string `json:"type"`
	TLS  string `json:"tls"`
	Add  string `json:"add"`
}

func parseVMess(uri string) (ProxyConfig, error) {
	raw := strings.TrimPrefix(uri, "vmess://")
	dec, err := decodeSubscriptionBase64(raw)
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("decode vmess: %w", err)
	}
	var v vmessJSON
	if err := json.Unmarshal([]byte(dec), &v); err != nil {
		return ProxyConfig{}, fmt.Errorf("parse vmess json: %w", err)
	}

	px := ProxyConfig{
		Type:   "vmess",
		Server: v.Add,
		UUID:   v.ID,
	}
	px.Port, _ = strconv.Atoi(v.Port)
	px.AlterID = v.AID
	px.Security = "auto"
	if v.TLS == "tls" {
		px.Security = "tls"
	}

	if v.PS != "" {
		name, hostname := extractName(v.PS)
		px.Name = name
		if hostname != "" {
			px.Server = hostname
		}
	}
	return px, nil
}

func decodeSubscriptionBase64(s string) (string, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		dec, err := enc.DecodeString(s)
		if err == nil {
			return string(dec), nil
		}
	}
	return "", fmt.Errorf("could not decode base64")
}
