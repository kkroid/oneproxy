package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const ProxySourceSubscription = "subscription"

type SubscriptionMergeResult struct {
	Added          int
	Updated        int
	Missing        int
	Removed        int
	Changed        bool
	RuntimeChanged bool
}

func MergeSubscription(current *Config, incoming []ProxyConfig, subscriptionURL string) (*Config, SubscriptionMergeResult, error) {
	var result SubscriptionMergeResult
	if current == nil {
		return nil, result, fmt.Errorf("current config is required")
	}
	if subscriptionURL == "" {
		return nil, result, fmt.Errorf("subscription URL is required")
	}
	if len(incoming) == 0 {
		return nil, result, fmt.Errorf("subscription contains no supported proxies")
	}

	next, err := cloneConfig(current)
	if err != nil {
		return nil, result, err
	}
	prepared, err := prepareSubscriptionProxies(incoming)
	if err != nil {
		return nil, result, err
	}

	sourceKey := subscriptionSourceKey(subscriptionURL)
	urlChanged := current.SubscriptionSourceKey != "" && current.SubscriptionSourceKey != sourceKey
	allowLegacyMatch := !current.SubscriptionMigrated && current.SubscriptionURL == subscriptionURL
	matchedCurrent := make(map[int]bool)
	matchedIncoming := make(map[int]int)

	if !urlChanged {
		for incomingIndex, proxy := range prepared {
			currentIndex, found, err := findSubscriptionMatch(current.Proxies, proxy, matchedCurrent, allowLegacyMatch)
			if err != nil {
				return nil, result, err
			}
			if found {
				matchedCurrent[currentIndex] = true
				matchedIncoming[incomingIndex] = currentIndex
			}
		}
	}

	merged := make([]ProxyConfig, 0, len(current.Proxies)+len(prepared))
	for currentIndex, existing := range current.Proxies {
		incomingIndex := -1
		for candidateIncoming, candidateCurrent := range matchedIncoming {
			if candidateCurrent == currentIndex {
				incomingIndex = candidateIncoming
				break
			}
		}
		if incomingIndex >= 0 {
			updated := prepared[incomingIndex]
			updated.LocalPort = existing.LocalPort
			updated.Enabled = existing.Enabled
			merged = append(merged, updated)
			if !reflect.DeepEqual(existing, updated) {
				if !proxyRuntimeEqual(existing, updated) {
					result.RuntimeChanged = true
				}
				result.Updated++
			}
			continue
		}

		if existing.Source == ProxySourceSubscription {
			if urlChanged {
				result.Removed++
				continue
			}
			existing.MissingUpdates++
			if existing.MissingUpdates >= 2 {
				result.Removed++
				result.RuntimeChanged = true
				continue
			}
			result.Missing++
		}
		merged = append(merged, existing)
	}

	usedPorts := make(map[int]bool)
	if current.Unified.Port > 0 {
		usedPorts[current.Unified.Port] = true
	}
	for _, proxy := range merged {
		if proxy.LocalPort > 0 {
			usedPorts[proxy.LocalPort] = true
		}
	}
	for incomingIndex, proxy := range prepared {
		if _, matched := matchedIncoming[incomingIndex]; matched {
			continue
		}
		proxy.LocalPort = nextAvailablePort(usedPorts, 10801)
		if proxy.LocalPort == 0 {
			return nil, result, fmt.Errorf("no local port is available for subscription proxy %q", proxy.Name)
		}
		proxy.Enabled = true
		usedPorts[proxy.LocalPort] = true
		merged = append(merged, proxy)
		result.Added++
		result.RuntimeChanged = true
	}

	next.Proxies = merged
	next.SubscriptionURL = subscriptionURL
	next.SubscriptionMigrated = true
	next.SubscriptionSourceKey = sourceKey
	if err := validateUniqueProxyNames(next.Proxies); err != nil {
		return nil, result, err
	}
	if err := next.Validate(); err != nil {
		return nil, result, fmt.Errorf("merged subscription config is invalid: %w", err)
	}
	result.Changed = !reflect.DeepEqual(current, next)
	return next, result, nil
}

func subscriptionSourceKey(subscriptionURL string) string {
	hash := sha256.Sum256([]byte(subscriptionURL))
	return hex.EncodeToString(hash[:16])
}

func MergeManualProxy(current *Config, incoming ProxyConfig) (*Config, error) {
	if current == nil {
		return nil, fmt.Errorf("current config is required")
	}
	next, err := cloneConfig(current)
	if err != nil {
		return nil, err
	}
	incoming.Source = "manual"
	incoming.SubscriptionKey = ""
	incoming.MissingUpdates = 0
	incoming.Enabled = true
	if incoming.Name == "" {
		incoming.Name = incoming.Server
	}

	for index, existing := range next.Proxies {
		if existing.Source != ProxySourceSubscription &&
			strings.EqualFold(existing.Server, incoming.Server) && existing.Port == incoming.Port {
			incoming.Name = existing.Name
			incoming.LocalPort = existing.LocalPort
			next.Proxies[index] = incoming
			if err := validateUniqueProxyNames(next.Proxies); err != nil {
				return nil, err
			}
			if err := next.Validate(); err != nil {
				return nil, err
			}
			return next, nil
		}
	}

	usedPorts := map[int]bool{}
	if next.Unified.Port > 0 {
		usedPorts[next.Unified.Port] = true
	}
	for _, proxy := range next.Proxies {
		if proxy.LocalPort > 0 {
			usedPorts[proxy.LocalPort] = true
		}
	}
	incoming.LocalPort = nextAvailablePort(usedPorts, 10801)
	if incoming.LocalPort == 0 {
		return nil, fmt.Errorf("no local port is available for manual proxy %q", incoming.Name)
	}
	next.Proxies = append(next.Proxies, incoming)
	if err := validateUniqueProxyNames(next.Proxies); err != nil {
		return nil, err
	}
	if err := next.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

func validateUniqueProxyNames(proxies []ProxyConfig) error {
	names := make(map[string]string)
	for _, proxy := range proxies {
		normalized := normalizedProxyName(proxy)
		if existing, found := names[normalized]; found {
			return fmt.Errorf("proxy name %q conflicts with %q", proxy.Name, existing)
		}
		names[normalized] = proxy.Name
	}
	return nil
}

func proxyRuntimeEqual(left, right ProxyConfig) bool {
	left.Source, left.SubscriptionKey, left.MissingUpdates = "", "", 0
	right.Source, right.SubscriptionKey, right.MissingUpdates = "", "", 0
	return reflect.DeepEqual(left, right)
}

func prepareSubscriptionProxies(proxies []ProxyConfig) ([]ProxyConfig, error) {
	nameCounts := make(map[string]int)
	for _, proxy := range proxies {
		if normalized := normalizedProxyName(proxy); normalized != "" {
			nameCounts[proxy.Type+"|"+normalized]++
		}
	}

	prepared := make([]ProxyConfig, len(proxies))
	keys := make(map[string]bool)
	for index, proxy := range proxies {
		identity := proxy.Type + "|endpoint|" + strings.ToLower(proxy.Server) + "|" + strconv.Itoa(proxy.Port)
		if normalized := normalizedProxyName(proxy); normalized != "" && nameCounts[proxy.Type+"|"+normalized] == 1 {
			identity = proxy.Type + "|name|" + normalized
		}
		hash := sha256.Sum256([]byte(identity))
		proxy.Source = ProxySourceSubscription
		proxy.SubscriptionKey = hex.EncodeToString(hash[:16])
		proxy.MissingUpdates = 0
		proxy.LocalPort = 0
		if keys[proxy.SubscriptionKey] {
			return nil, fmt.Errorf("subscription contains duplicate proxy identity for %q", proxy.Name)
		}
		keys[proxy.SubscriptionKey] = true
		prepared[index] = proxy
	}
	return prepared, nil
}

func findSubscriptionMatch(proxies []ProxyConfig, incoming ProxyConfig, used map[int]bool, allowLegacy bool) (int, bool, error) {
	matchBy := func(predicate func(ProxyConfig) bool) (int, bool, error) {
		matched := -1
		for index, proxy := range proxies {
			if used[index] || (!allowLegacy && proxy.Source != ProxySourceSubscription) || !predicate(proxy) {
				continue
			}
			if proxy.Source != "" && proxy.Source != ProxySourceSubscription {
				continue
			}
			if matched >= 0 {
				return -1, false, fmt.Errorf("subscription proxy %q matches multiple existing proxies", incoming.Name)
			}
			matched = index
		}
		return matched, matched >= 0, nil
	}

	if index, found, err := matchBy(func(proxy ProxyConfig) bool {
		return proxy.Source == ProxySourceSubscription && proxy.SubscriptionKey == incoming.SubscriptionKey
	}); found || err != nil {
		return index, found, err
	}
	if index, found, err := matchBy(func(proxy ProxyConfig) bool {
		return proxy.Type == incoming.Type && normalizedProxyName(proxy) != "" && normalizedProxyName(proxy) == normalizedProxyName(incoming)
	}); found || err != nil {
		return index, found, err
	}
	return matchBy(func(proxy ProxyConfig) bool {
		return proxy.Type == incoming.Type && strings.EqualFold(proxy.Server, incoming.Server) && proxy.Port == incoming.Port
	})
}

func normalizedProxyName(proxy ProxyConfig) string {
	return strings.ToLower(strings.TrimSpace(proxy.Name))
}

func nextAvailablePort(used map[int]bool, start int) int {
	for port := start; port <= 65535; port++ {
		if !used[port] {
			return port
		}
	}
	return 0
}

func cloneConfig(current *Config) (*Config, error) {
	data, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	var clone Config
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}
