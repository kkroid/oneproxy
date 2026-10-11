package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"
)

func TestMergeSubscriptionSharedEndpointDistinctNames(t *testing.T) {
	for _, differentAccount := range []bool{false, true} {
		for _, migration := range []string{"fresh", "legacy", "name-key", "endpoint-key"} {
			t.Run(fmt.Sprintf("%s/different-account=%v", migration, differentAccount), func(t *testing.T) {
				current := subscriptionTestConfig()
				current.Proxies = current.Proxies[:1]
				a := ProxyConfig{Name: "Plan A", Type: "shadowsocks", Server: "shared.example.com", Port: 443, Method: "aes-256-gcm", Password: "synthetic-a", LocalPort: 10831}
				b := a
				b.Name, b.LocalPort, b.Enabled = "Plan B", 10832, true
				if differentAccount {
					b.Password = "synthetic-b"
				}
				if migration != "fresh" {
					current.Proxies = append(current.Proxies, a, b)
					if migration == "endpoint-key" {
						current.Proxies = current.Proxies[:2] // r1 supported just one node per endpoint.
					}
					if migration != "legacy" {
						current.SubscriptionMigrated = true
						for i := 1; i < len(current.Proxies); i++ {
							proxy := &current.Proxies[i]
							identity := proxy.Type + "|name|" + normalizedProxyName(*proxy)
							if migration == "endpoint-key" {
								identity = proxy.Type + "|endpoint|shared.example.com|443"
							}
							hash := sha256.Sum256([]byte(identity))
							proxy.Source, proxy.SubscriptionKey = ProxySourceSubscription, hex.EncodeToString(hash[:16])
						}
					}
				}
				next, _, err := MergeSubscription(current, []ProxyConfig{b, a, b}, current.SubscriptionURL)
				if err != nil {
					t.Fatal(err)
				}
				if len(next.Proxies) != 3 || !reflect.DeepEqual(next.Proxies[0], current.Proxies[0]) {
					t.Fatal("shared endpoint entries were lost or manual node changed")
				}
				for _, proxy := range next.Proxies[1:] {
					want := a
					if proxy.Name == b.Name {
						want = b
					}
					if proxy.Name != want.Name || proxy.Password != want.Password {
						t.Fatal("shared endpoint accounts were mixed")
					}
					if migration != "fresh" && (migration != "endpoint-key" || proxy.Name == a.Name) && (proxy.LocalPort != want.LocalPort || proxy.Enabled != want.Enabled) {
						t.Fatal("migration moved port or enabled state between accounts")
					}
				}
				for i := 0; i < 3; i++ {
					incoming := []ProxyConfig{a, b}
					if i%2 == 0 {
						incoming = []ProxyConfig{b, a}
					}
					again, result, err := MergeSubscription(next, incoming, current.SubscriptionURL)
					if err != nil || result.Changed || !reflect.DeepEqual(next, again) {
						t.Fatalf("shared endpoint repeat/reorder drift: %+v, %v", result, err)
					}
				}
			})
		}
	}
}

func TestMergeSubscriptionRemovesBundledExample(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		current, err := Load("../../config-placeholder.json")
		if err != nil {
			t.Fatal(err)
		}
		incoming := []ProxyConfig{subscriptionTestConfig().Proxies[1]}
		url := "https://subscription.example.com"
		if migrated {
			prepared, err := prepareSubscriptionProxies(incoming)
			if err != nil {
				t.Fatal(err)
			}
			prepared[0].Enabled = true
			prepared[0].LocalPort = 10802
			current.Proxies = append(current.Proxies, prepared[0])
			current.SubscriptionURL = url
			current.SubscriptionSourceKey = subscriptionSourceKey(url)
			current.SubscriptionMigrated = true
		}
		before, _ := cloneConfig(current)
		next, result, err := MergeSubscription(current, incoming, url)
		if err != nil {
			t.Fatal(err)
		}
		if len(next.Proxies) != 1 || next.Proxies[0].Name != incoming[0].Name || result.Removed != 1 || !result.Changed || !result.RuntimeChanged {
			t.Fatalf("migrated=%v: example not removed: %+v, %+v", migrated, next.Proxies, result)
		}
		wantPort := 10801
		if migrated {
			wantPort = 10802
		}
		if next.Proxies[0].LocalPort != wantPort {
			t.Fatalf("port = %d, want %d", next.Proxies[0].LocalPort, wantPort)
		}
		if !reflect.DeepEqual(current, before) {
			t.Fatal("merge mutated the original config")
		}
		_, again, err := MergeSubscription(next, incoming, url)
		if err != nil || again.Changed {
			t.Fatalf("repeat update = %+v, %v", again, err)
		}
	}
}

func TestMergeSubscriptionPreservesCustomizedExample(t *testing.T) {
	for _, field := range []string{"server", "password", "name", "source", "extra"} {
		t.Run(field, func(t *testing.T) {
			current, err := Load("../../config-placeholder.json")
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "server":
				current.Proxies[0].Server = "real.example.org"
			case "password":
				current.Proxies[0].Password = "custom-password"
			case "name":
				current.Proxies[0].Name = "My Node"
			case "source":
				current.Proxies[0].Source = "manual"
			case "extra":
				current.Proxies[0].Extra = map[string]interface{}{"custom": true}
			}
			next, result, err := MergeSubscription(current, []ProxyConfig{subscriptionTestConfig().Proxies[1]}, "https://subscription.example.com")
			if err != nil {
				t.Fatal(err)
			}
			if len(next.Proxies) != 2 || result.Removed != 0 || !reflect.DeepEqual(current.Proxies[0], next.Proxies[0]) {
				t.Fatalf("customized example changed: %+v", next.Proxies)
			}
		})
	}
}

func TestMergeSubscriptionInvalidImportKeepsExample(t *testing.T) {
	current, err := Load("../../config-placeholder.json")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := cloneConfig(current)
	for _, incoming := range [][]ProxyConfig{nil, {{Name: "Invalid", Type: "unsupported"}}} {
		if _, _, err := MergeSubscription(current, incoming, "https://subscription.example.com"); err == nil {
			t.Fatal("invalid import succeeded")
		}
		if !reflect.DeepEqual(current, before) {
			t.Fatal("failed import changed the original config")
		}
	}
}

func TestMergeSubscriptionDoesNotMatchExampleAsLegacyNode(t *testing.T) {
	current, err := Load("../../config-placeholder.json")
	if err != nil {
		t.Fatal(err)
	}
	current.SubscriptionURL = "https://subscription.example.com"
	current.Proxies[0].Enabled = false
	current.Proxies[0].LocalPort = 10820
	incoming := subscriptionTestConfig().Proxies[1]
	incoming.Name = "Example-Server"
	next, result, err := MergeSubscription(current, []ProxyConfig{incoming}, current.SubscriptionURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Proxies) != 1 || result.Removed != 1 || result.Added != 1 || !next.Proxies[0].Enabled || next.Proxies[0].LocalPort != 10801 || next.Proxies[0].Server != incoming.Server {
		t.Fatalf("real subscription node inherited placeholder state: %+v, %+v", next.Proxies, result)
	}
}

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

func TestMergeSubscriptionPreservesManualNameConflict(t *testing.T) {
	current := subscriptionTestConfig()
	current.Proxies[0].Source = "manual"
	next, _, err := MergeSubscription(current, []ProxyConfig{{
		Name: "Manual", Type: "shadowsocks", Server: "subscription.example.com", Port: 443,
		Method: "aes-256-gcm", Password: "password",
	}}, current.SubscriptionURL)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next.Proxies[0], current.Proxies[0]) || next.Proxies[len(next.Proxies)-1].Name == "Manual" {
		t.Fatal("manual node was changed or subscription name was not disambiguated")
	}
}

func TestMergeSubscriptionNameCollisionsStable(t *testing.T) {
	a := subscriptionTestConfig().Proxies[1]
	a.Name = "Shared"
	b := a
	b.Server = "b.example.com"
	b.Name = " shared "
	for _, legacy := range []bool{false, true} {
		current := subscriptionTestConfig()
		current.Proxies = current.Proxies[:1]
		if legacy {
			current.Proxies = append(current.Proxies, a)
		}
		// New endpoint comes first: it must not steal the legacy endpoint's state.
		incoming := []ProxyConfig{b, a, a}
		next, _, err := MergeSubscription(current, incoming, current.SubscriptionURL)
		if err != nil {
			t.Fatal(err)
		}
		if len(next.Proxies) != 3 {
			t.Fatalf("duplicate was not collapsed: %d", len(next.Proxies))
		}
		if legacy && (next.Proxies[1].Server != a.Server || next.Proxies[1].Name != a.Name || next.Proxies[1].Enabled || next.Proxies[1].LocalPort != a.LocalPort) {
			t.Fatal("legacy state moved to another endpoint")
		}
		for i := 0; i < 3; i++ {
			incoming[0], incoming[1] = incoming[1], incoming[0]
			again, result, err := MergeSubscription(next, incoming, current.SubscriptionURL)
			if err != nil || result.Changed || !reflect.DeepEqual(next, again) {
				t.Fatalf("repeat/reorder drift: %+v, %v", result, err)
			}
		}
	}
}

func TestMergeSubscriptionOldNameKeyAndSuffixCollision(t *testing.T) {
	current := subscriptionTestConfig()
	a := current.Proxies[1]
	oldHash := sha256.Sum256([]byte(a.Type + "|name|" + normalizedProxyName(a)))
	current.Proxies[1].Source = ProxySourceSubscription
	current.Proxies[1].SubscriptionKey = hex.EncodeToString(oldHash[:16])
	current.SubscriptionMigrated = true
	manual := current.Proxies[0]
	manual.Name, manual.LocalPort = "Node A (2)", 10821
	current.Proxies = append(current.Proxies, manual)
	b := a
	b.Server = "b.example.com"
	for _, incoming := range [][]ProxyConfig{{b, a}, {a, b}} {
		next, _, err := MergeSubscription(current, incoming, current.SubscriptionURL)
		if err != nil {
			t.Fatal(err)
		}
		if next.Proxies[1].Server != a.Server || next.Proxies[1].Name != a.Name || next.Proxies[1].Enabled || next.Proxies[1].LocalPort != a.LocalPort || next.Proxies[3].Name != "Node A (3)" {
			t.Fatal("old name identity or suffix collision was not preserved")
		}
		if !reflect.DeepEqual(next.Proxies[2], manual) {
			t.Fatal("manual suffix owner changed")
		}
		current = next
	}
}

func TestMergeSubscriptionRejectsAmbiguousEndpointWithoutMutation(t *testing.T) {
	current := subscriptionTestConfig()
	before, _ := cloneConfig(current)
	a := current.Proxies[1]
	b := a
	b.Password = "different-account"
	if _, _, err := MergeSubscription(current, []ProxyConfig{a, b}, current.SubscriptionURL); err == nil {
		t.Fatal("conflicting accounts on one identity must not silently overwrite")
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatal("failed merge changed original config")
	}
}

func TestMergeSubscriptionDistinctEndpointsSameName(t *testing.T) {
	for _, name := range []string{"Shared", " shared "} {
		current := subscriptionTestConfig()
		current.Proxies = current.Proxies[:1]
		a := current.Proxies[0]
		a.Name, a.Server = "Shared", "a.example.com"
		b := a
		b.Name, b.Server = name, "b.example.com"
		first, _, err := MergeSubscription(current, []ProxyConfig{a, b}, current.SubscriptionURL)
		if err != nil {
			t.Fatal(err)
		}
		reversed, _, err := MergeSubscription(current, []ProxyConfig{b, a}, current.SubscriptionURL)
		if err != nil || !reflect.DeepEqual(first, reversed) {
			t.Fatal("initial order changed allocated names or ports")
		}
	}
}

func TestMergeSubscriptionReservesEndpointBeforeNameFallback(t *testing.T) {
	current := subscriptionTestConfig()
	a := current.Proxies[1]
	b := a
	b.Server = "b.example.com"
	a.Name = "Renamed"
	next, _, err := MergeSubscription(current, []ProxyConfig{b, a}, current.SubscriptionURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Proxies) != 3 || next.Proxies[1].Server != a.Server || next.Proxies[1].Name != "Node A" || next.Proxies[1].Enabled || next.Proxies[1].LocalPort != a.LocalPort {
		t.Fatal("weak name match stole the existing endpoint's state")
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
