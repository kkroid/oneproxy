package config

import (
	"crypto/sha256"
	"fmt"
	"net"
	"reflect"
)

// SubscriptionUpdateMessages describes a committed update without logging feed
// URLs, names, addresses or credentials. Hashes allow correlation across updates.
func SubscriptionUpdateMessages(before, after *Config) []string {
	status := "updated"
	if reflect.DeepEqual(before, after) {
		status = "unchanged"
	}
	messages := []string{fmt.Sprintf("subscription update succeeded: status=%s proxies=%d", status, len(after.Proxies))}
	for _, old := range before.Proxies {
		for _, next := range after.Proxies {
			if old.Name == next.Name && (old.Server != next.Server || old.Port != next.Port) {
				messages = append(messages, fmt.Sprintf("subscription address changed: node=%x old=%s new=%s",
					diagnosticHash(old.Name), addressDiagnostic(old), addressDiagnostic(next)))
				break
			}
		}
	}
	return messages
}

func diagnosticHash(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:8]
}

func addressDiagnostic(proxy ProxyConfig) string {
	kind := "domain"
	if ip := net.ParseIP(proxy.Server); ip != nil {
		kind = "ipv6"
		if ip.To4() != nil {
			kind = "ipv4"
		}
	}
	return fmt.Sprintf("%s:%x", kind, diagnosticHash(fmt.Sprintf("%s:%d", proxy.Server, proxy.Port)))
}
