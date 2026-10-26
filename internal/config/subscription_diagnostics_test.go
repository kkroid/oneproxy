package config

import (
	"strings"
	"testing"
)

func TestSubscriptionUpdateMessagesRedactAndDistinguishChanges(t *testing.T) {
	before := &Config{SubscriptionURL: "https://private.invalid/?token=secret", Proxies: []ProxyConfig{{
		Name: "private-name", Server: "192.0.2.123", Port: 443, UUID: "private-uuid",
		Password: "private-password", RealityPublicKey: "private-key",
	}}}
	after, err := cloneConfig(before)
	if err != nil {
		t.Fatal(err)
	}
	if messages := SubscriptionUpdateMessages(before, after); len(messages) != 1 || !strings.Contains(messages[0], "status=unchanged") {
		t.Fatal(messages)
	}
	after.Proxies[0].Server = "c702s1.portablesubmarines.com"
	messages := SubscriptionUpdateMessages(before, after)
	joined := strings.Join(messages, "\n")
	if len(messages) != 2 || !strings.Contains(joined, "status=updated") || !strings.Contains(joined, "old=ipv4:") || !strings.Contains(joined, "new=domain:") {
		t.Fatal(messages)
	}
	for _, secret := range []string{"private", "secret", before.Proxies[0].Server, after.Proxies[0].Server} {
		if strings.Contains(joined, secret) {
			t.Fatal("diagnostic exposed input")
		}
	}
}
