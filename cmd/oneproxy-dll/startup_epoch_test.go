package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func startAtEpochForTest(path string, epoch uint64) error {
	gMu.Lock()
	defer gMu.Unlock()
	return startProxyAtEpoch(path, epoch)
}

func TestStartupEpochStopBeforeDLLAndDuringDownload(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		close(entered)
		<-release // A late server response must not save or start after Stop.
		feed(w)
	}))
	defer server.Close()
	defer close(release)
	root, path, before := startupFixture(t, server.URL)
	stale := uint64(OneProxy_StartupEpoch())
	OneProxy_Stop()
	if err := startAtEpochForTest(path, stale); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("queued startup survived Stop: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("stale startup downloaded after Stop")
	}

	// A new user Start is allowed. An old queued call must not cancel or adopt it.
	fresh := uint64(OneProxy_StartupEpoch())
	if stale == fresh {
		t.Fatal("Stop did not invalidate startup epoch")
	}
	done := make(chan error, 1)
	go func() { done <- startAtEpochForTest(path, fresh) }()
	awaitSubscription(t, entered)
	gMu.Lock()
	active := gStartup
	gMu.Unlock()
	if err := startAtEpochForTest(path, stale); err == nil {
		t.Fatal("stale startup entered replacement session")
	}
	gMu.Lock()
	same := active != nil && gStartup == active
	gMu.Unlock()
	if !same {
		t.Fatal("stale call changed replacement session")
	}
	OneProxy_Stop()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "canceled") {
			t.Fatalf("download survived Stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not cancel in-flight download")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("canceled startup changed configuration")
	}
	if _, err := os.Stat(filepath.Join(root, ".oneproxy", "singbox_generated.json")); !os.IsNotExist(err) {
		t.Fatal("canceled startup generated runtime configuration")
	}
	gMu.Lock()
	defer gMu.Unlock()
	if gStartup != nil || gManager != nil || gConfig != nil || subscriptionLastErr != "" {
		t.Fatal("canceled startup published state or connected")
	}
	if requests.Load() != 1 {
		t.Fatal("unexpected duplicate or stale download")
	}
}
