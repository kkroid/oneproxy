package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
)

func TestSubscriptionRejectsResponseFromPreviousSession(t *testing.T) {
	old := gConfig
	t.Cleanup(func() { gConfig = old })
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("vless://test-id@a.example:443?type=tcp&security=reality&flow=xtls-rprx-vision&sni=cover.example&fp=chrome&pbk=test-key&sid=abcd#Test")))
	}))
	defer server.Close()
	gConfig = &config.Config{SubscriptionURL: server.URL}
	done := make(chan error, 1)
	go func() { done <- updateSubscription(server.URL, false) }()
	<-entered
	gMu.Lock()
	replacement := &config.Config{SubscriptionURL: server.URL}
	gConfig = replacement
	gMu.Unlock()
	close(release)
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "session changed") {
			t.Fatalf("old response accepted: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscription update blocked")
	}
	if gConfig != replacement || len(gConfig.Proxies) != 0 {
		t.Fatal("previous session modified new configuration")
	}
	stop := make(chan struct{})
	close(stop)
	if err := updateConfiguredSubscriptionForSession(stop); err == nil {
		t.Fatal("stopped scheduled updater ran")
	}
}
