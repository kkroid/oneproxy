package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kkroid/oneproxy/internal/config"
)

func TestSubscriptionUpdateCollisionsAndFailurePreservation(t *testing.T) {
	for _, failure := range []string{"", "fetch", "merge", "save", "restart"} {
		t.Run("failure="+failure, func(t *testing.T) {
			dir := t.TempDir()
			oldConfig, oldDir, oldRestart := gConfig, subscriptionDataDir, subscriptionRestart
			oldClient, oldAPI := selectorHTTPClient, clashAPIBaseURL
			t.Cleanup(func() {
				gConfig, subscriptionDataDir, subscriptionRestart = oldConfig, oldDir, oldRestart
				selectorHTTPClient, clashAPIBaseURL = oldClient, oldAPI
			})
			subscriptionDataDir = func() string { return dir }
			restarts := 0
			subscriptionRestart = func() error {
				restarts++
				if failure == "restart" && restarts == 1 {
					return fmt.Errorf("synthetic restart failure")
				}
				return nil
			}
			line := func(host, name string) string {
				return "vless://test-id@" + host + ":443?type=tcp&security=reality&flow=xtls-rprx-vision&sni=cover.example&fp=chrome&pbk=test-key&sid=abcd#" + name
			}
			a := line("a.example", "Shared")
			body := a + "\n" + line("b.example", "Shared") + "\n" + a
			if failure == "merge" {
				body += "\n" + strings.Replace(a, "test-id@", "conflicting-account@", 1)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if failure == "fetch" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte(body)))
			}))
			defer server.Close()
			selected := make(chan string, 4)
			selector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					var payload map[string]string
					_ = json.NewDecoder(r.Body).Decode(&payload)
					selected <- payload["name"]
					w.WriteHeader(http.StatusNoContent)
					return
				}
				fmt.Fprint(w, `{"now":"out-Shared"}`)
			}))
			defer selector.Close()
			selectorHTTPClient, clashAPIBaseURL = selector.Client(), selector.URL
			proxy, err := config.ParseSubscriptionLine(a)
			if err != nil {
				t.Fatal(err)
			}
			proxy.LocalPort, proxy.Enabled = 10808, true
			gConfig = &config.Config{Version: "1.0", Unified: config.UnifiedConfig{Port: 1082}, SubscriptionURL: server.URL, Proxies: []config.ProxyConfig{proxy}}
			original := gConfig
			beforeMemory, _ := json.Marshal(original)
			path := filepath.Join(dir, "config.json")
			if err := original.Save(path); err != nil {
				t.Fatal(err)
			}
			beforeFile, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "save" {
				// A regular file cannot contain config.json. The original stays untouched.
				subscriptionDataDir = func() string { return path }
			}
			err = updateSubscription(server.URL, false)
			if failure != "" {
				if err == nil {
					t.Fatal("expected synthetic failure")
				}
				afterMemory, _ := json.Marshal(gConfig)
				afterFile, readErr := os.ReadFile(path)
				if readErr != nil || gConfig != original || !bytes.Equal(beforeMemory, afterMemory) || !bytes.Equal(beforeFile, afterFile) {
					t.Fatal("failed update changed memory or config file")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(gConfig.Proxies) != 2 || gConfig.Proxies[0].Name != "Shared" || gConfig.Proxies[0].LocalPort != 10808 || !gConfig.Proxies[0].Enabled {
				t.Fatal("existing selected node was not preserved")
			}
			select {
			case tag := <-selected:
				if tag != "out-Shared" {
					t.Fatalf("selection moved to %q", tag)
				}
			default:
				t.Fatal("manual selection was not restored")
			}
			first := gConfig
			if err := updateSubscription(server.URL, false); err != nil || !reflect.DeepEqual(first, gConfig) || restarts != 1 {
				t.Fatalf("repeat update drift/restart: %v, %d", err, restarts)
			}
		})
	}
}
