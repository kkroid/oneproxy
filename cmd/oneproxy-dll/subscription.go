package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
)

const subscriptionInterval = 6 * time.Hour

type subscriptionSession struct {
	stop chan struct{}
	done chan struct{}
}

var (
	subscriptionDataDir  = resolveDataDir
	subscriptionRestart  = reloadAndRestart
	subscriptionUpdateMu sync.Mutex
	// Session and published status are protected by gMu, like Start and Stop.
	subscriptionActive   *subscriptionSession
	subscriptionUpdating bool
	subscriptionLastAt   time.Time
	subscriptionLastErr  string
	subscriptionResultID uint64
)

// Requires gMu; only called after the proxy has started successfully.
func startSubscriptionUpdater() {
	if subscriptionActive != nil {
		return
	}
	session := &subscriptionSession{stop: make(chan struct{}), done: make(chan struct{})}
	subscriptionActive = session
	go func() {
		defer close(session.done)
		ticker := time.NewTicker(subscriptionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = updateConfiguredSubscriptionForSession(session.stop)
			case <-session.stop:
				return
			}
		}
	}()
}

// Requires gMu. It never waits for a request or takes subscriptionUpdateMu.
func stopSubscriptionUpdater() {
	if subscriptionActive != nil {
		close(subscriptionActive.stop)
		subscriptionActive = nil
	}
	subscriptionUpdating = false
	subscriptionLastAt = time.Time{}
	subscriptionLastErr = ""
}

func updateConfiguredSubscription() error {
	return updateConfiguredSubscriptionForSession(nil)
}

func updateConfiguredSubscriptionForSession(stop <-chan struct{}) error {
	gMu.Lock()
	defer gMu.Unlock()
	select {
	case <-stop:
		return fmt.Errorf("subscription session stopped")
	default:
	}
	if gConfig == nil || gConfig.SubscriptionURL == "" {
		return fmt.Errorf("subscription URL not configured")
	}
	return updateSubscriptionLocked(gConfig.SubscriptionURL, false)
}

func updateSubscription(url string, allowSourceChange bool) error {
	gMu.Lock()
	defer gMu.Unlock()
	return updateSubscriptionLocked(url, allowSourceChange)
}

// Called with gMu held; drops it for queuing and download, returns with it held.
// Capture identity BEFORE queuing so an old caller cannot adopt a new session.
func updateSubscriptionLocked(url string, allowSourceChange bool) error {
	if gStartup != nil {
		return fmt.Errorf("startup in progress; try again after startup")
	}
	current, session := gConfig, subscriptionActive
	if current == nil {
		return fmt.Errorf("not started")
	}
	var stop <-chan struct{}
	if session != nil {
		stop = session.stop
	}
	gMu.Unlock()
	subscriptionUpdateMu.Lock()
	defer subscriptionUpdateMu.Unlock()
	gMu.Lock()
	if gConfig != current || subscriptionActive != session {
		return fmt.Errorf("subscription session changed while updating")
	}
	if !allowSourceChange && current.SubscriptionURL != url {
		return fmt.Errorf("configured subscription changed while updating")
	}
	subscriptionUpdating = true
	subscriptionLastErr = ""
	gMu.Unlock()
	incoming, err := fetchSubscriptionForSession(url, stop)
	gMu.Lock()
	if subscriptionActive == session {
		subscriptionUpdating = false
	}
	if gConfig != current || subscriptionActive != session {
		return fmt.Errorf("subscription session changed while updating")
	}
	if err != nil {
		return setSubscriptionUpdateError()
	}
	next, result, err := config.MergeSubscription(current, incoming, url)
	if err != nil {
		return setSubscriptionUpdateError()
	}
	if !result.Changed {
		logSubscriptionUpdate(current, next)
		setSubscriptionUpdateSuccess()
		return nil
	}

	selectionMode, selectedProxy := selectorState(current)
	configPath := filepath.Join(subscriptionDataDir(), "config.json")
	if !result.RuntimeChanged {
		if err := next.Save(configPath); err != nil {
			return setSubscriptionUpdateError()
		}
		gConfig = next
		logSubscriptionUpdate(current, next)
		setSubscriptionUpdateSuccess()
		return nil
	}
	gConfig = next
	if err := subscriptionRestart(); err != nil {
		gConfig = current
		_ = subscriptionRestart()
		restoreSubscriptionSelection(current, selectionMode, selectedProxy)
		return setSubscriptionUpdateError()
	}
	// Persist only after runtime application succeeds; a restart failure must not
	// rewrite even the formatting of the original config file.
	if err := next.Save(configPath); err != nil {
		gConfig = current
		_ = subscriptionRestart()
		restoreSubscriptionSelection(current, selectionMode, selectedProxy)
		return setSubscriptionUpdateError()
	}
	restoreSubscriptionSelection(next, selectionMode, selectedProxy)
	if gLogger != nil {
		gLogger.Info("subscription updated: added=%d updated=%d missing=%d removed=%d",
			result.Added, result.Updated, result.Missing, result.Removed)
	}
	logSubscriptionUpdate(current, next)
	setSubscriptionUpdateSuccess()
	return nil
}

// FetchSubscription owns a 15s HTTP timeout and the shared feed parser. Stop
// releases the update queue immediately; a late download only owns this buffered
// result channel and cannot access configuration, state, or a replacement session.
func fetchSubscriptionForSession(url string, stop <-chan struct{}) ([]config.ProxyConfig, error) {
	if stop == nil {
		proxies, _, err := config.FetchSubscription(url, 0)
		return proxies, err
	}
	select {
	case <-stop:
		return nil, fmt.Errorf("subscription session stopped")
	default:
	}
	type response struct {
		proxies []config.ProxyConfig
		err     error
	}
	result := make(chan response, 1)
	go func() {
		proxies, _, err := config.FetchSubscription(url, 0)
		result <- response{proxies, err}
	}()
	select {
	case <-stop:
		return nil, fmt.Errorf("subscription session stopped")
	case received := <-result:
		return received.proxies, received.err
	}
}

func restoreSubscriptionSelection(cfg *config.Config, mode, name string) {
	if mode != "manual" || !proxyEnabledByName(cfg, name) {
		return
	}
	selectorTag := cfg.Unified.Tag
	if selectorTag == "" {
		selectorTag = "proxy"
	}
	outboundTag, err := selectionOutboundTag(name, cfg.Proxies)
	if err == nil {
		if err := restoreSelectorSelection(selectorTag, outboundTag); err != nil && gLogger != nil {
			gLogger.Warn("could not restore manual proxy selection")
		}
	}
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

// Requires gMu. Do not expose feed contents, URLs, or parser errors in status.
func setSubscriptionUpdateError() error {
	subscriptionLastErr = "Subscription update failed. Please check the subscription and try again."
	subscriptionResultID++
	if gLogger != nil {
		gLogger.Warn("subscription update failed")
	}
	return fmt.Errorf("%s", subscriptionLastErr)
}

func setSubscriptionUpdateSuccess() {
	subscriptionLastAt = time.Now()
	subscriptionLastErr = ""
	subscriptionResultID++
}

// Requires gMu so subscription status and the proxy session form one snapshot.
func subscriptionStatus() (bool, time.Time, string, string) {
	resultID := ""
	if subscriptionLastErr != "" || !subscriptionLastAt.IsZero() {
		resultID = strconv.FormatUint(subscriptionResultID, 10)
	}
	return subscriptionUpdating, subscriptionLastAt, subscriptionLastErr, resultID
}

func logSubscriptionUpdate(before, after *config.Config) {
	if gLogger != nil {
		for _, message := range config.SubscriptionUpdateMessages(before, after) {
			gLogger.Info("%s", message)
		}
	}
}
