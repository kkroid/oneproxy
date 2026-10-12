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

func awaitSubscription(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscription operation did not complete")
	}
}

func isolatedSubscriptionState(t *testing.T) {
	t.Helper()
	gMu.Lock()
	old := gConfig
	stopSubscriptionUpdater()
	gMu.Unlock()
	t.Cleanup(func() {
		gMu.Lock()
		session := subscriptionActive
		stopSubscriptionUpdater()
		gConfig = old
		gMu.Unlock()
		if session != nil {
			awaitSubscription(t, session.done)
		}
	})
}

func TestSubscriptionPeriodicUpdaterDoesNotRefreshImmediately(t *testing.T) {
	entered := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { entered <- struct{}{} }))
	defer server.Close()
	isolatedSubscriptionState(t)
	gMu.Lock()
	gConfig = &config.Config{SubscriptionURL: server.URL}
	startSubscriptionUpdater()
	first := subscriptionActive
	startSubscriptionUpdater()
	if subscriptionActive != first {
		t.Fatal("duplicate updater")
	}
	gMu.Unlock()
	select {
	case <-entered:
		t.Fatal("startup performed duplicate refresh")
	case <-time.After(100 * time.Millisecond):
	}
	gMu.Lock()
	stopSubscriptionUpdater()
	gMu.Unlock()
	awaitSubscription(t, first.done)
	if subscriptionInterval != 6*time.Hour {
		t.Fatal("periodic interval changed")
	}
}

func TestPeriodicSessionStopRejectsLateFailure(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	defer close(release)
	isolatedSubscriptionState(t)
	gMu.Lock()
	gConfig = &config.Config{SubscriptionURL: server.URL}
	startSubscriptionUpdater()
	previous := subscriptionActive
	gMu.Unlock()
	done := make(chan error, 1)
	go func() { done <- updateConfiguredSubscriptionForSession(previous.stop) }()
	awaitSubscription(t, entered)
	gMu.Lock()
	stopSubscriptionUpdater()
	replacement := &config.Config{}
	gConfig = replacement
	startSubscriptionUpdater()
	current := subscriptionActive
	gMu.Unlock()
	awaitSubscription(t, previous.done)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stopped update succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stopped update stuck")
	}
	gMu.Lock()
	defer gMu.Unlock()
	if subscriptionActive != current || gConfig != replacement || subscriptionUpdating || subscriptionLastErr != "" || !subscriptionLastAt.IsZero() {
		t.Fatal("stopped periodic session polluted replacement")
	}
}

func TestSubscriptionQueuedRequestKeepsOriginalSession(t *testing.T) {
	isolatedSubscriptionState(t)
	original := &config.Config{SubscriptionURL: "http://old.invalid"}
	gMu.Lock()
	gConfig = original
	// Hold the queue and launch while holding gMu, like the real entrypoint.
	subscriptionUpdateMu.Lock()
	done := make(chan error, 1)
	go func() {
		err := updateSubscriptionLocked(original.SubscriptionURL, false)
		gMu.Unlock()
		done <- err
	}()
	// Acquiring gMu proves the request captured its identity and entered the queue.
	gMu.Lock()
	replacement := &config.Config{SubscriptionURL: "http://new.invalid"}
	gConfig = replacement
	gMu.Unlock()
	subscriptionUpdateMu.Unlock()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "session changed") {
			t.Fatalf("queued old request accepted: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued request stuck")
	}
	gMu.Lock()
	defer gMu.Unlock()
	if gConfig != replacement || subscriptionUpdating || subscriptionLastErr != "" {
		t.Fatal("queued request mutated new session")
	}
}

func TestSubscriptionFailureStatusIsSanitizedAndVersioned(t *testing.T) {
	isolatedSubscriptionState(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Malformed supported URL causes the shared parser to include its input.
		fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("vless://secret:password@%invalid?token=credential")))
	}))
	defer server.Close()
	gMu.Lock()
	gConfig = &config.Config{SubscriptionURL: server.URL + "/?token=private"}
	gMu.Unlock()
	previous := ""
	for i := 0; i < 2; i++ {
		err := updateConfiguredSubscription()
		if err == nil {
			t.Fatal("expected parse failure")
		}
		gMu.Lock()
		updating, lastAt, message, id := subscriptionStatus()
		gMu.Unlock()
		if updating || !lastAt.IsZero() || message == "" || id == "" || id == previous {
			t.Fatal("missing distinct failure result")
		}
		for _, secret := range []string{"secret", "password", "credential", "private", server.URL} {
			if strings.Contains(message, secret) || strings.Contains(err.Error(), secret) {
				t.Fatal("credentials exposed in result")
			}
		}
		previous = id
	}
	gMu.Lock()
	stopSubscriptionUpdater()
	updating, at, message, id := subscriptionStatus()
	gMu.Unlock()
	if updating || !at.IsZero() || message != "" || id != "" {
		t.Fatal("stopped status not cleared")
	}
}

func TestSubscriptionUnconfiguredDoesNotPublishResult(t *testing.T) {
	isolatedSubscriptionState(t)
	gMu.Lock()
	gConfig = &config.Config{}
	gMu.Unlock()
	if err := updateConfiguredSubscription(); err == nil {
		t.Fatal("unconfigured request succeeded")
	}
	gMu.Lock()
	defer gMu.Unlock()
	if subscriptionUpdating || subscriptionLastErr != "" || !subscriptionLastAt.IsZero() {
		t.Fatal("unconfigured update published a result")
	}
}
