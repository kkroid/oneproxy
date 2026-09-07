package main

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
)

const (
	subscriptionInitialDelay = 30 * time.Second
	subscriptionInterval     = 6 * time.Hour
)

var (
	subscriptionUpdateMu    sync.Mutex
	subscriptionLifecycleMu sync.Mutex
	subscriptionStop        chan struct{}
	subscriptionStateMu     sync.RWMutex
	subscriptionUpdating    bool
	subscriptionLastAt      time.Time
	subscriptionLastErr     string
)

func startSubscriptionUpdater() {
	subscriptionLifecycleMu.Lock()
	if subscriptionStop != nil {
		close(subscriptionStop)
	}
	stop := make(chan struct{})
	subscriptionStop = stop
	subscriptionLifecycleMu.Unlock()
	go func() {
		timer := time.NewTimer(subscriptionInitialDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
			_ = updateConfiguredSubscription()
		case <-stop:
			return
		}

		ticker := time.NewTicker(subscriptionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = updateConfiguredSubscription()
			case <-stop:
				return
			}
		}
	}()
}

func stopSubscriptionUpdater() {
	subscriptionLifecycleMu.Lock()
	defer subscriptionLifecycleMu.Unlock()
	if subscriptionStop != nil {
		close(subscriptionStop)
		subscriptionStop = nil
	}
}

func updateConfiguredSubscription() error {
	gMu.Lock()
	if gConfig == nil || gConfig.SubscriptionURL == "" {
		gMu.Unlock()
		return fmt.Errorf("subscription URL not configured")
	}
	url := gConfig.SubscriptionURL
	gMu.Unlock()
	return updateSubscription(url, false)
}

func updateSubscription(url string, allowSourceChange bool) error {
	subscriptionUpdateMu.Lock()
	defer subscriptionUpdateMu.Unlock()
	setSubscriptionUpdateState(true, "")
	defer setSubscriptionUpdating(false)

	incoming, _, err := config.FetchSubscription(url, 0)
	if err != nil {
		setSubscriptionUpdateError(err)
		return err
	}

	gMu.Lock()
	defer gMu.Unlock()
	if gConfig == nil {
		err := fmt.Errorf("not started")
		setSubscriptionUpdateError(err)
		return err
	}
	if !allowSourceChange && gConfig.SubscriptionURL != url {
		err := fmt.Errorf("configured subscription changed while updating")
		setSubscriptionUpdateError(err)
		return err
	}
	current := gConfig
	next, result, err := config.MergeSubscription(current, incoming, url)
	if err != nil {
		setSubscriptionUpdateError(err)
		return err
	}
	if !result.Changed {
		setSubscriptionUpdateSuccess()
		return nil
	}

	selectionMode, selectedProxy := selectorState(current)
	configPath := filepath.Join(resolveDataDir(), "config.json")
	if err := next.Save(configPath); err != nil {
		setSubscriptionUpdateError(err)
		return err
	}
	gConfig = next
	if !result.RuntimeChanged {
		setSubscriptionUpdateSuccess()
		return nil
	}
	if err := reloadAndRestart(); err != nil {
		gConfig = current
		_ = current.Save(configPath)
		_ = reloadAndRestart()
		setSubscriptionUpdateError(err)
		return err
	}
	if selectionMode == "manual" && proxyEnabledByName(next, selectedProxy) {
		selectorTag := next.Unified.Tag
		if selectorTag == "" {
			selectorTag = "proxy"
		}
		outboundTag, err := selectionOutboundTag(selectedProxy, next.Proxies)
		if err == nil {
			if err := restoreSelectorSelection(selectorTag, outboundTag); err != nil && gLogger != nil {
				gLogger.Warn("could not restore manual proxy selection: %v", err)
			}
		}
	}
	if gLogger != nil {
		gLogger.Info("subscription updated: added=%d updated=%d missing=%d removed=%d",
			result.Added, result.Updated, result.Missing, result.Removed)
	}
	setSubscriptionUpdateSuccess()
	return nil
}

func restoreSelectorSelection(selectorTag, outboundTag string) error {
	var lastError error
	for attempt := 0; attempt < 10; attempt++ {
		lastError = putSelectorSelection(selectorHTTPClient, clashAPIBaseURL, selectorTag, outboundTag)
		if lastError == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return lastError
}

func proxyEnabledByName(cfg *config.Config, name string) bool {
	for _, proxy := range cfg.Proxies {
		if proxy.Enabled && proxy.Name == name {
			return true
		}
	}
	return false
}

func setSubscriptionUpdateState(updating bool, lastError string) {
	subscriptionStateMu.Lock()
	defer subscriptionStateMu.Unlock()
	subscriptionUpdating = updating
	subscriptionLastErr = lastError
}

func setSubscriptionUpdating(updating bool) {
	subscriptionStateMu.Lock()
	subscriptionUpdating = updating
	subscriptionStateMu.Unlock()
}

func setSubscriptionUpdateError(err error) {
	subscriptionStateMu.Lock()
	subscriptionLastErr = err.Error()
	subscriptionStateMu.Unlock()
	if gLogger != nil {
		gLogger.Warn("subscription update failed: %v", err)
	}
}

func setSubscriptionUpdateSuccess() {
	subscriptionStateMu.Lock()
	subscriptionLastAt = time.Now()
	subscriptionLastErr = ""
	subscriptionStateMu.Unlock()
}

func subscriptionStatus() (bool, time.Time, string) {
	subscriptionStateMu.RLock()
	defer subscriptionStateMu.RUnlock()
	return subscriptionUpdating, subscriptionLastAt, subscriptionLastErr
}
