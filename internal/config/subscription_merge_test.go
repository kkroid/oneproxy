package config

import "testing"

func TestMergeSubscriptionPreservesManualNodesAndPorts(t *testing.T) {
	current := subscriptionTestConfig()
	incoming := []ProxyConfig{
		{Name: "Node A", Type: "shadowsocks", Server: "new-a.example.com", Port: 443, Method: "aes-256-gcm", Password: "new-password"},
		{Name: "Node B", Type: "vless", Server: "b.example.com", Port: 443, UUID: "new-uuid", Security: "reality", Flow: "xtls-rprx-vision", ServerName: "cover.example.com", Fingerprint: "chrome", RealityPublicKey: "new-key", RealityShortID: "new-id"},
	}

	next, result, err := MergeSubscription(current, incoming, "https://subscription.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Added != 1 || result.Updated != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(next.Proxies) != 3 {
		t.Fatalf("len(proxies) = %d, want 3", len(next.Proxies))
	}
	if next.Proxies[0].Source != "" || next.Proxies[0].LocalPort != 10820 {
		t.Fatalf("manual proxy changed: %+v", next.Proxies[0])
	}
	if next.Proxies[1].LocalPort != 10801 || next.Proxies[1].Password != "new-password" || next.Proxies[1].Enabled {
		t.Fatalf("existing subscription proxy was not merged correctly: %+v", next.Proxies[1])
	}
	if next.Proxies[2].LocalPort != 10802 || !next.Proxies[2].Enabled {
		t.Fatalf("new subscription proxy port = %+v", next.Proxies[2])
	}
}

func TestMergeSubscriptionDeletesAfterTwoMissingUpdates(t *testing.T) {
	current := subscriptionTestConfig()
	other := ProxyConfig{Name: "Other", Enabled: true, LocalPort: 10802, Type: "shadowsocks",
		Server: "other.example.com", Port: 443, Method: "aes-256-gcm", Password: "password"}
	prepared, err := prepareSubscriptionProxies([]ProxyConfig{current.Proxies[1], other})
	if err != nil {
		t.Fatal(err)
	}
	current.Proxies[1].Source = ProxySourceSubscription
	current.Proxies[1].SubscriptionKey = prepared[0].SubscriptionKey
	other.Source = ProxySourceSubscription
	other.SubscriptionKey = prepared[1].SubscriptionKey
	current.Proxies = append(current.Proxies, other)
	current.SubscriptionMigrated = true

	first, result, err := MergeSubscription(current, []ProxyConfig{{
		Name: "Other", Type: "shadowsocks", Server: "other.example.com", Port: 443,
		Method: "aes-256-gcm", Password: "password",
	}}, current.SubscriptionURL)
	if err != nil {
		t.Fatal(err)
	}
	if result.Missing != 1 || result.Removed != 0 || len(first.Proxies) != 3 {
		t.Fatalf("first missing update = %+v, proxies=%d", result, len(first.Proxies))
	}
	if result.RuntimeChanged {
		t.Fatal("first missing update should not require a proxy restart")
	}

	second, result, err := MergeSubscription(first, []ProxyConfig{{
		Name: "Other", Type: "shadowsocks", Server: "other.example.com", Port: 443,
		Method: "aes-256-gcm", Password: "password",
	}}, current.SubscriptionURL)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || len(second.Proxies) != 2 {
		t.Fatalf("second missing update = %+v, proxies=%d", result, len(second.Proxies))
	}
	if !result.RuntimeChanged {
		t.Fatal("removing a proxy must require a proxy restart")
	}
}

func TestMergeSubscriptionMigratesLegacyMatchingNodes(t *testing.T) {
	current := subscriptionTestConfig()
	incoming := []ProxyConfig{{
		Name: "Node A", Type: "shadowsocks", Server: "a.example.com", Port: 443,
		Method: "aes-256-gcm", Password: "new-password",
	}}

	next, _, err := MergeSubscription(current, incoming, current.SubscriptionURL)
	if err != nil {
		t.Fatal(err)
	}
	if next.Proxies[1].Source != ProxySourceSubscription || next.Proxies[1].LocalPort != 10801 {
		t.Fatalf("legacy proxy was not migrated: %+v", next.Proxies[1])
	}
	if !next.SubscriptionMigrated {
		t.Fatal("legacy migration was not recorded")
	}
}

func TestMergeSubscriptionURLChangeReplacesOnlySubscriptionNodes(t *testing.T) {
	current := subscriptionTestConfig()
	prepared, err := prepareSubscriptionProxies([]ProxyConfig{current.Proxies[1]})
	if err != nil {
		t.Fatal(err)
	}
	current.Proxies[1].Source = ProxySourceSubscription
	current.Proxies[1].SubscriptionKey = prepared[0].SubscriptionKey
	current.SubscriptionMigrated = true
	current.SubscriptionSourceKey = subscriptionSourceKey(current.SubscriptionURL)

	next, result, err := MergeSubscription(current, []ProxyConfig{{
		Name: "Node A", Type: "shadowsocks", Server: "new.example.com", Port: 443,
		Method: "aes-256-gcm", Password: "new-password",
	}}, "https://new-subscription.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || result.Added != 1 || len(next.Proxies) != 2 {
		t.Fatalf("URL change result = %+v, proxies=%d", result, len(next.Proxies))
	}
	if next.Proxies[0].Name != "Manual" || next.Proxies[1].Server != "new.example.com" {
		t.Fatalf("unexpected proxies after URL change: %+v", next.Proxies)
	}
}

func TestMergeSubscriptionRejectsManualNameConflict(t *testing.T) {
	current := subscriptionTestConfig()
	current.Proxies[0].Source = "manual"
	_, _, err := MergeSubscription(current, []ProxyConfig{{
		Name: "Manual", Type: "shadowsocks", Server: "subscription.example.com", Port: 443,
		Method: "aes-256-gcm", Password: "password",
	}}, current.SubscriptionURL)
	if err == nil {
		t.Fatal("MergeSubscription() accepted a manual/subscription name conflict")
	}
}

func TestMergeManualProxyPreservesSubscriptionNodes(t *testing.T) {
	current := subscriptionTestConfig()
	current.Proxies[1].Source = ProxySourceSubscription
	next, err := MergeManualProxy(current, ProxyConfig{
		Name: "Manual Two", Type: "shadowsocks", Server: "manual-two.example.com", Port: 443,
		Method: "aes-256-gcm", Password: "password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Proxies) != 3 || next.Proxies[1].Source != ProxySourceSubscription {
		t.Fatalf("subscription proxy changed: %+v", next.Proxies)
	}
	if next.Proxies[2].Source != "manual" || next.Proxies[2].LocalPort != 10802 {
		t.Fatalf("manual proxy not merged correctly: %+v", next.Proxies[2])
	}
}

func subscriptionTestConfig() *Config {
	return &Config{
		Version:         "1.0",
		Unified:         UnifiedConfig{Port: 1082},
		SubscriptionURL: "https://subscription.example.com",
		Proxies: []ProxyConfig{
			{Name: "Manual", Enabled: true, LocalPort: 10820, Type: "shadowsocks", Server: "manual.example.com", Port: 443, Method: "aes-256-gcm", Password: "password"},
			{Name: "Node A", Enabled: false, LocalPort: 10801, Type: "shadowsocks", Server: "a.example.com", Port: 443, Method: "aes-256-gcm", Password: "old-password"},
		},
	}
}
