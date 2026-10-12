package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionHTTPTimeoutAndCancellation(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(map[bool]string{false: "15s timeout", true: "cancel"}[cancelEarly], func(t *testing.T) {
			entered, canceled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(canceled) }))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			start := time.Now()
			go func() { _, _, err := FetchSubscriptionContext(ctx, server.URL+"/?token=secret", 0); done <- err }()
			<-entered
			if cancelEarly {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil || strings.Contains(err.Error(), "secret") {
					t.Fatalf("unsafe/missing error: %v", err)
				}
				if !cancelEarly && time.Since(start) < 14*time.Second {
					t.Fatal("did not exercise real HTTP timeout")
				}
			case <-time.After(18 * time.Second):
				t.Fatal("request exceeded timeout")
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("transport not canceled")
			}
		})
	}
}

func TestSubscriptionRejectsOversizedFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("A", (1<<20)+1))) }))
	defer server.Close()
	if _, _, err := FetchSubscription(server.URL, 0); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("oversized feed accepted: %v", err)
	}
}
