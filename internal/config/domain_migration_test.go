package config

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func domainTestURI(fragment string) string {
	return strings.Split(strings.Replace(testVLESSURI, "edge.example.com", "192.0.2.7", 1), "#")[0] + "#" + url.PathEscape(fragment)
}

func TestVLESSConfirmedRemarkChangesOnlyServer(t *testing.T) {
	for _, tc := range []struct{ fragment, server string }{
		{"JMS-123@c702s2.portablesubmarines.com:443", "c702s2.portablesubmarines.com"},
		{"JMS-456@c702s801.portablesubmarines.com:443", "c702s801.portablesubmarines.com"},
		{"c702s2.portablesubmarines.com", "192.0.2.7"},
		{"JMS-123@c702s2.portablesubmarines.com:8443", "192.0.2.7"},
		{"JMS-123@c702s2.portablesubmarines.com:99999999999999999999", "192.0.2.7"},
		{"JMS-123@c702s2.portablesubmarines.com", "192.0.2.7"},
		{"JMS-name@c702s2.portablesubmarines.com:443", "192.0.2.7"},
		{"JMS-123@c702s2.portablesubmarines.com.evil.test:443", "192.0.2.7"},
		{"JMS-123@other.example.com:443", "192.0.2.7"},
		{"JMS-123@c702sx.portablesubmarines.com:443", "192.0.2.7"},
		{"JMS-123@C702s2.portablesubmarines.com:443", "192.0.2.7"},
		{"prefix JMS-123@c702s2.portablesubmarines.com:443", "192.0.2.7"},
		{"JMS-123@c702s2.portablesubmarines.com:443\n", "192.0.2.7"},
		{"JMS-123%40c702s2.portablesubmarines.com%3A443", "192.0.2.7"},
	} {
		t.Run(tc.fragment, func(t *testing.T) {
			baseline, err := ParseSubscriptionLine(domainTestURI(""))
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseSubscriptionLine(domainTestURI(tc.fragment))
			if err != nil {
				t.Fatal(err)
			}
			baseline.Name, baseline.Server = extractHostname(tc.fragment), tc.server
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("unexpected server or changed connection fields")
			}
		})
	}
}

func TestDomainMigrationPreservesLocalStateAndIsIdempotent(t *testing.T) {
	for _, state := range []string{"legacy", "tracked", "renamed"} {
		t.Run(state, func(t *testing.T) {
			incoming, err := ParseSubscriptionLine(domainTestURI("JMS-123@c702s2.portablesubmarines.com:443"))
			if err != nil {
				t.Fatal(err)
			}
			old := incoming
			old.Server, old.LocalPort, old.Enabled = "192.0.2.99", 10888, false
			if state != "legacy" {
				old.Source, old.SubscriptionKey = ProxySourceSubscription, subscriptionProxyKey(old)
			}
			if state == "renamed" {
				old.Name = "My selected node"
			}
			current := &Config{Version: "1.0", SubscriptionURL: "https://subscription.invalid", SubscriptionMigrated: state != "legacy", Proxies: []ProxyConfig{old}}
			before, _ := cloneConfig(current)
			next, result, err := MergeSubscription(current, []ProxyConfig{incoming, incoming}, current.SubscriptionURL)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(current, before) || len(next.Proxies) != 1 || result.Added != 0 || result.Updated != 1 || !result.RuntimeChanged {
				t.Fatalf("migration drift: %+v", result)
			}
			want := incoming
			want.Name, want.LocalPort, want.Enabled = old.Name, old.LocalPort, old.Enabled
			want.Source, want.SubscriptionKey = ProxySourceSubscription, subscriptionProxyKey(incoming)
			if !reflect.DeepEqual(next.Proxies[0], want) {
				t.Fatal("migration did not preserve local state/connection fields")
			}
			for i := 0; i < 3; i++ {
				again, result, err := MergeSubscription(next, []ProxyConfig{incoming}, current.SubscriptionURL)
				if err != nil || result.Changed || !reflect.DeepEqual(next, again) {
					t.Fatalf("repeat update drift: %+v %v", result, err)
				}
			}
		})
	}
}

func TestDomainMigrationRejectsAmbiguousOldKeys(t *testing.T) {
	incoming, err := ParseSubscriptionLine(domainTestURI("JMS-123@c702s2.portablesubmarines.com:443"))
	if err != nil {
		t.Fatal(err)
	}
	a, b := incoming, incoming
	a.Server, b.Server = "192.0.2.10", "192.0.2.20"
	a.Source, b.Source = ProxySourceSubscription, ProxySourceSubscription
	a.SubscriptionKey, b.SubscriptionKey = subscriptionProxyKey(a), subscriptionProxyKey(b)
	// Previously imported same-name entries have unique local names and ports.
	a.Name, b.Name, a.LocalPort, b.LocalPort = "Plan A", "Plan B", 10801, 10802
	current := &Config{Version: "1.0", SubscriptionMigrated: true, SubscriptionURL: "https://feed.invalid", Proxies: []ProxyConfig{a, b}}
	before, _ := cloneConfig(current)
	if next, _, err := MergeSubscription(current, []ProxyConfig{incoming}, current.SubscriptionURL); err == nil || next != nil || !reflect.DeepEqual(current, before) {
		t.Fatal("ambiguous domain migration must fail without changing the old config")
	}
}

func TestDomainMigrationDoesNotAddGenericIPIdentityMatching(t *testing.T) {
	incoming, err := ParseSubscriptionLine(domainTestURI("JMS-123@other.example:443"))
	if err != nil {
		t.Fatal(err)
	}
	old := incoming
	old.Server, old.Source = "192.0.2.99", ProxySourceSubscription
	old.SubscriptionKey = subscriptionProxyKey(old)
	old.Name, old.LocalPort = "Renamed ordinary node", 10888
	current := &Config{Version: "1.0", SubscriptionMigrated: true, SubscriptionURL: "https://feed.invalid", Proxies: []ProxyConfig{old}}
	next, result, err := MergeSubscription(current, []ProxyConfig{incoming}, current.SubscriptionURL)
	if err != nil || result.Added != 1 || result.Missing != 1 || len(next.Proxies) != 2 || next.Proxies[0].Server != old.Server {
		t.Fatal("migration rule expanded to an unrelated node")
	}
}
