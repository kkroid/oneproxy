package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
)

// The real Manager launches this test executable, never a real proxy. Its
// snapshot proves what was on disk at the exact process launch boundary.
func TestMain(m *testing.M) {
	if os.Getenv("ONEPROXY_STARTUP_HELPER") == "1" && len(os.Args) > 1 && os.Args[1] == "run" {
		generated, err := os.ReadFile(os.Args[len(os.Args)-1])
		if err != nil {
			os.Exit(2)
		}
		saved, err := os.ReadFile(os.Getenv("ONEPROXY_STARTUP_CONFIG"))
		if err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Getenv("ONEPROXY_STARTUP_SNAPSHOT"), append(append(saved, '\n'), generated...), 0600); err != nil {
			os.Exit(4)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	os.Exit(m.Run())
}

func startupFixture(t *testing.T, url string) (string, string, []byte) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("HOME", root)
	path := filepath.Join(root, "config.json")
	cfg := &config.Config{Version: "1.0", SubscriptionURL: url, Proxies: []config.ProxyConfig{{Name: "Old", Enabled: true, Type: "shadowsocks", Server: "old.example", Port: 443, Method: "aes-256-gcm", Password: "old"}}}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	t.Cleanup(func() {
		OneProxy_Stop()
		if gLogger != nil {
			gLogger.Close()
			gLogger = nil
		}
	})
	return root, path, before
}

func startFixture(path, root string) error {
	gMu.Lock()
	defer gMu.Unlock()
	return startProxyInDataDir(path, root)
}

func feed(w http.ResponseWriter) {
	fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("vless://test-id@new.example:443?type=tcp&security=reality&flow=xtls-rprx-vision&sni=cover.example&fp=chrome&pbk=test-key&sid=abcd#Fresh")))
}

func installStartupHelper(t *testing.T, root, path string) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("DLL process fixture uses Windows direct child lifecycle")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err = os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bin, singBoxExecutableName()), data, 0755); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, "launched.txt")
	t.Setenv("ONEPROXY_STARTUP_HELPER", "1")
	t.Setenv("ONEPROXY_STARTUP_CONFIG", path)
	t.Setenv("ONEPROXY_STARTUP_SNAPSHOT", snapshot)
	return snapshot
}

func TestActualStartupSavesBeforeLaunchingAndRefreshesEveryStart(t *testing.T) {
	var requests atomic.Int32
	var fail atomic.Bool
	entered, release := make(chan struct{}, 8), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		entered <- struct{}{}
		<-release
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		feed(w)
	}))
	defer server.Close()
	root, path, before := startupFixture(t, server.URL)
	snapshot := installStartupHelper(t, root, path)
	done := make(chan error, 1)
	go func() { done <- startFixture(path, root) }()
	awaitSubscription(t, entered)
	current, _ := os.ReadFile(path)
	if !bytes.Equal(before, current) {
		t.Fatal("saved before download completed")
	}
	if _, err := os.Stat(snapshot); !os.IsNotExist(err) {
		t.Fatal("launched old configuration")
	}
	if _, err := os.Stat(filepath.Join(root, "singbox_generated.json")); !os.IsNotExist(err) {
		t.Fatal("generated before update")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	launched, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(launched, []byte("new.example")) != 2 {
		t.Fatalf("saved and generated configs not both updated: %s", launched)
	}
	if requests.Load() != 1 {
		t.Fatal("duplicate startup refresh")
	}
	OneProxy_Stop()
	if err := startFixture(path, root); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatal("second start did not refresh exactly once")
	}
	if err := OneProxy_Restart(); err != nil {
		OneProxy_FreeString(err)
		t.Fatal("restart failed")
	}
	if requests.Load() != 3 {
		t.Fatal("explicit Restart bypassed update")
	}
	fail.Store(true)
	if err := OneProxy_Restart(); err == nil {
		t.Fatal("failed refresh restarted old nodes")
	} else {
		OneProxy_FreeString(err)
	}
	if gManager != nil || requests.Load() != 4 {
		t.Fatal("failed Restart retained a connection")
	}
}

func TestActualStartupFailurePreservesFileAndNeverLaunches(t *testing.T) {
	for _, kind := range []string{"fetch", "parse", "merge", "validate", "save"} {
		t.Run(kind, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-release
				switch kind {
				case "fetch":
					w.WriteHeader(503)
				case "parse":
					fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("vless://secret@%bad?token=private")))
				case "merge":
					fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("ss://YWVzLTI1Ni1nY206YQ==@new.example:443#Same\nss://YWVzLTI1Ni1nY206Yg==@new.example:443#Same")))
				case "validate":
					fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("ss://YWVzLTI1Ni1nY206YQ==@new.example:0#Invalid")))
				default:
					feed(w)
				}
			}))
			defer server.Close()
			root, path, before := startupFixture(t, server.URL+"/?token=private")
			done := make(chan error, 1)
			go func() { done <- startFixture(path, root) }()
			awaitSubscription(t, entered)
			// Turn the destination into a nonempty directory while preserving the
			// original bytes, forcing atomic replacement failure on every platform.
			original := path
			if kind == "save" {
				original = path + ".original"
				if err := os.Rename(path, original); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "blocked"), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			close(release)
			err := <-done
			if err == nil || !strings.Contains(err.Error(), "not connected") || strings.Contains(err.Error(), "private") {
				t.Fatalf("unsafe/missing failure: %v", err)
			}
			after, _ := os.ReadFile(original)
			if !bytes.Equal(before, after) {
				t.Fatal("original bytes changed")
			}
			if gManager != nil || gConfig.Proxies[0].Server != "old.example" {
				t.Fatal("failure launched or changed memory")
			}
			if _, err := os.Stat(filepath.Join(root, "singbox_generated.json")); !os.IsNotExist(err) {
				t.Fatal("failure generated partial runtime config")
			}
		})
	}
}

func TestActualStartupStopConcurrentUpdatesAndReplacement(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; feed(w) }))
	defer server.Close()
	defer close(release)
	root, path, before := startupFixture(t, server.URL)
	done := make(chan error, 1)
	go func() { done <- startFixture(path, root) }()
	awaitSubscription(t, entered)
	if err := startFixture(path, root); err == nil {
		t.Fatal("concurrent Start accepted")
	}
	if err := updateConfiguredSubscription(); err == nil {
		t.Fatal("manual update accepted during startup")
	}
	if err := updateConfiguredSubscriptionForSession(nil); err == nil {
		t.Fatal("periodic update accepted during startup")
	}
	if err := importManualProxy(config.ProxyConfig{}); err == nil {
		t.Fatal("manual import accepted during startup")
	}
	OneProxy_Stop()
	replacement := &config.Config{Version: "replacement"}
	gMu.Lock()
	gConfig = replacement
	gMu.Unlock()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled startup succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not cancel download")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("canceled session saved")
	}
	gMu.Lock()
	defer gMu.Unlock()
	if gConfig != replacement || gManager != nil || subscriptionLastErr != "" {
		t.Fatal("old session changed replacement")
	}
}

func TestFreshInstallWithoutSubscriptionStartsDirectly(t *testing.T) {
	root, path, before := startupFixture(t, "")
	snapshot := installStartupHelper(t, root, path)
	if err := os.WriteFile(filepath.Join(root, "config-placeholder.json"), before, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "fresh-data")
	target := filepath.Join(dataDir, "config.json")
	t.Setenv("ONEPROXY_STARTUP_CONFIG", target)
	if err := startFixture("config.json", dataDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, before) {
		t.Fatal("placeholder not recovered intact")
	}
}
