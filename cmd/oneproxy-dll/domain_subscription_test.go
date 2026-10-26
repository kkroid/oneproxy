package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kkroid/oneproxy/internal/config"
	"github.com/kkroid/oneproxy/internal/logger"
)

func TestDomainUpdatePreservesSelectionAndLogsAllOutcomes(t *testing.T) {
	isolatedSubscriptionState(t)
	oldDir, oldRestart, oldLogger := subscriptionDataDir, subscriptionRestart, gLogger
	oldClient, oldAPI := selectorHTTPClient, clashAPIBaseURL
	dir := t.TempDir()
	logPath := filepath.Join(dir, "update.log")
	var err error
	gLogger, err = logger.New(logPath, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		gLogger.Close()
		gLogger, subscriptionDataDir, subscriptionRestart = oldLogger, oldDir, oldRestart
		selectorHTTPClient, clashAPIBaseURL = oldClient, oldAPI
	})
	subscriptionDataDir = func() string { return dir }
	restarts := 0
	subscriptionRestart = func() error { restarts++; return nil }
	line := "vless://synthetic-uuid@192.0.2.8:443?type=tcp&security=reality&flow=xtls-rprx-vision&sni=cover.example&fp=chrome&pbk=synthetic-key&sid=abcd#JMS-123%40c702s2.portablesubmarines.com%3A443"
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "secret-token synthetic-uuid synthetic-key")
			return
		}
		fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte(line+"\n"+line)))
	}))
	defer server.Close()
	var selected atomic.Value
	selector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			var payload map[string]string
			_ = json.NewDecoder(r.Body).Decode(&payload)
			selected.Store(payload["name"])
			w.WriteHeader(http.StatusNoContent)
			return
		}
		fmt.Fprint(w, `{"now":"out-My-node"}`)
	}))
	defer selector.Close()
	selectorHTTPClient, clashAPIBaseURL = selector.Client(), selector.URL
	px, err := config.ParseSubscriptionLine(line)
	if err != nil {
		t.Fatal(err)
	}
	px.Server = "192.0.2.99" // old core used a different IP before the provider's update
	url := server.URL + "/?token=secret-token"
	gConfig, _, err = config.MergeSubscription(&config.Config{Version: "1.0"}, []config.ProxyConfig{px}, url)
	if err != nil {
		t.Fatal(err)
	}
	gConfig.Proxies[0].Name, gConfig.Proxies[0].LocalPort = "My node", 10888
	if err := updateConfiguredSubscription(); err != nil {
		t.Fatal(err)
	}
	if len(gConfig.Proxies) != 1 || gConfig.Proxies[0].Server != "c702s2.portablesubmarines.com" || gConfig.Proxies[0].Name != "My node" || gConfig.Proxies[0].LocalPort != 10888 || selected.Load() != "out-My-node" {
		t.Fatal("migration changed local identity or selection")
	}
	first := gConfig
	if err := updateConfiguredSubscription(); err != nil || restarts != 1 || !reflect.DeepEqual(first, gConfig) {
		t.Fatal("repeat migration changed runtime/config")
	}
	fail.Store(true)
	if err := updateConfiguredSubscription(); err == nil || gConfig != first || restarts != 1 {
		t.Fatal("failed update changed active config")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logs := string(data)
	for _, want := range []string{"status=updated", "status=unchanged", "subscription update failed", "old=ipv4:", "new=domain:"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing log %q", want)
		}
	}
	for _, secret := range []string{server.URL, "secret-token", "synthetic-uuid", "synthetic-key", "192.0.2.", "portablesubmarines", "My node"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log exposed %q", secret)
		}
	}
}
