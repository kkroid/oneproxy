package proxy

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
)

func TestHealthStopJoinsRecoveryAndRejectsQueuedRound(t *testing.T) {
	hc := NewHealthChecker(&config.Config{}, nil)
	hc.results["down"] = &HealthResult{LocalPort: 1}
	hc.allDownCount = 2
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	hc.SetAllDownCallback(func() {
		calls.Add(1)
		close(entered)
		<-release
	})
	go hc.CheckAll()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("recovery did not start")
	}
	queued := make(chan struct{})
	go func() { hc.CheckAll(); close(queued) }()
	stopped := make(chan struct{})
	go func() { hc.Stop(); close(stopped) }()
	select {
	case <-stopped:
		close(release)
		t.Fatal("Stop returned before recovery finished")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	for _, done := range []chan struct{}{stopped, queued} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("lifecycle did not finish")
		}
	}
	hc.CheckAll()
	if calls.Load() != 1 {
		t.Fatalf("stale recovery callbacks: %d", calls.Load())
	}
}
