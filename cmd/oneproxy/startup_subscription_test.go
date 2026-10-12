package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkroid/oneproxy/internal/config"
)

func TestCLIActualEntryUpdatesBeforeGenerationAndPreservesOnFailure(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("USERPROFILE", root)
			t.Setenv("HOME", root)
			path := filepath.Join(root, "config.json")
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if _, err := os.Stat(filepath.Join(root, ".oneproxy", "singbox_generated.json")); !os.IsNotExist(err) {
					t.Error("generated before fetch completed")
				}
				if !success {
					w.WriteHeader(503)
					return
				}
				fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("vless://test-id@new.example:443?type=tcp&security=reality&flow=xtls-rprx-vision&sni=cover.example&fp=chrome&pbk=test-key&sid=abcd#Fresh")))
			}))
			defer server.Close()
			cfg := &config.Config{Version: "1.0", SubscriptionURL: server.URL + "/?token=secret", Proxies: []config.ProxyConfig{{Name: "Old", Type: "shadowsocks", Server: "old.example", Port: 443, Method: "aes-256-gcm", Password: "old"}}}
			if err := cfg.Save(path); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			var out, stderr bytes.Buffer
			code := run([]string{"--config", path}, &out, &stderr)
			// The test executable has no adjacent sing-box: success must reach the
			// process check only AFTER saving and generation; failure must not reach it.
			if code != 1 || requests != 1 {
				t.Fatalf("run=%d requests=%d: %s", code, requests, stderr.String())
			}
			after, _ := os.ReadFile(path)
			generated, genErr := os.ReadFile(filepath.Join(root, ".oneproxy", "singbox_generated.json"))
			if success {
				if !bytes.Contains(after, []byte("new.example")) || !bytes.Contains(generated, []byte("new.example")) || !strings.Contains(stderr.String(), "check sing-box config") {
					t.Fatalf("incorrect ordering: %s", stderr.String())
				}
			} else if !bytes.Equal(before, after) || !os.IsNotExist(genErr) || !strings.Contains(stderr.String(), "not connected") || strings.Contains(stderr.String(), "secret") {
				t.Fatalf("unsafe failure: %s", stderr.String())
			}
		})
	}
}

func TestCLIStartupCancellationAndNoSubscription(t *testing.T) {
	cfg := &config.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if next, err := prepareCLIStartup(ctx, cfg, filepath.Join(t.TempDir(), "unused")); err != nil || next != cfg {
		t.Fatal("no-subscription fast path changed")
	}
	cfg.SubscriptionURL = "http://127.0.0.1:1/?token=secret"
	if _, err := prepareCLIStartup(ctx, cfg, filepath.Join(t.TempDir(), "unused")); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("cancellation not sanitized")
	}
}
