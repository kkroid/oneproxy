package proxy

import (
	"crypto/sha256"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// Only validated fields leave the private core log. Never copy an error or URL.
var (
	diagnosticNode = regexp.MustCompile(`outbound/[a-z0-9_-]+\[([^\]\r\n]+)\]`)
	diagnosticDial = regexp.MustCompile(`dial tcp (\[[0-9a-fA-F:]+\]|[0-9.]+):[0-9]+:`)
	diagnosticTime = regexp.MustCompile(`\[[0-9]+ ([0-9.]+(?:ms|µs|s|m))\]`)
)

func nodeDiagnosticID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", sum[:6])
}

func diagnosticFields(data []byte) string {
	node, ip, stage, elapsed := "unknown", "unknown", "unknown", "unknown"
	if match := diagnosticNode.FindSubmatch(data); match != nil {
		node = nodeDiagnosticID(string(match[1]))
	}
	if match := diagnosticDial.FindSubmatch(data); match != nil {
		if addr, err := netip.ParseAddr(strings.Trim(string(match[1]), "[]")); err == nil {
			ip, stage = addr.String(), "tcp_connect"
		}
	}
	if coreFailureKind(data) == "dns_resolution_failed" {
		stage = "dns"
	}
	if match := diagnosticTime.FindSubmatch(data); match != nil {
		if duration, err := time.ParseDuration(string(match[1])); err == nil {
			elapsed = duration.String()
		}
	}
	return fmt.Sprintf("node_id=%s dial_ip=%s stage=%s elapsed=%s", node, ip, stage, elapsed)
}

func healthFailureKind(message string) string {
	if kind := coreFailureKind([]byte(message)); kind != "" {
		return kind
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"):
		return "timeout"
	case strings.HasPrefix(message, "HTTP "):
		return "http_status"
	default:
		return "unknown"
	}
}
