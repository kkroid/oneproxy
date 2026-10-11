package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kkroid/oneproxy/internal/proxy"
)

func TestDNSManualEntryErrorsAndSerialization(t *testing.T) {
	oldManager, oldFlusher := gManager, gDNSFlusher
	t.Cleanup(func() { gManager, gDNSFlusher = oldManager, oldFlusher })
	for _, tt := range []struct {
		name    string
		manager *proxy.Manager
		flusher *proxy.DNSFlusher
		result  error
	}{
		{"missing manager", nil, proxy.NewDNSFlusher(), nil},
		{"missing flusher", &proxy.Manager{}, nil, nil},
		{"success", &proxy.Manager{}, proxy.NewDNSFlusher(), nil},
		{"system failure", &proxy.Manager{}, proxy.NewDNSFlusher(), errors.New("system DNS failure")},
		{"restart failure", &proxy.Manager{}, proxy.NewDNSFlusher(), errors.New("restart failure")},
		{"core stopped during flush", &proxy.Manager{}, proxy.NewDNSFlusher(), errors.New("system DNS flushed, but core stopped")},
		{"cooldown", &proxy.Manager{}, proxy.NewDNSFlusher(), errors.New("cooldown")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gManager, gDNSFlusher = tt.manager, tt.flusher
			calls := 0
			err := flushDNS(func(flusher *proxy.DNSFlusher, manager *proxy.Manager) error {
				calls++
				if flusher != tt.flusher || manager != tt.manager {
					t.Error("manual entry did not use current state")
				}
				if gMu.TryLock() {
					gMu.Unlock()
					t.Error("manual operation must hold the same mutex as Stop")
				}
				return tt.result
			})
			if tt.manager == nil || tt.flusher == nil {
				if err == nil || calls != 0 {
					t.Fatalf("nil state: err=%v calls=%d", err, calls)
				}
			} else if !errors.Is(err, tt.result) || calls != 1 {
				t.Fatalf("manual operation: err=%v want=%v calls=%d", err, tt.result, calls)
			}
			if !gMu.TryLock() {
				t.Fatal("manual entry did not release mutex")
			}
			gMu.Unlock()
		})
	}
}

func TestDNSDLLWiring(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	code := strings.ReplaceAll(string(source), "\r\n", "\n")
	for _, forbidden := range []string{".SetAllDownCallback(", ".FlushSystemDNS(", ".CanFlush("} {
		if strings.Contains(code, forbidden) {
			t.Fatalf("unexpected DNS wiring: %s", forbidden)
		}
	}
	for _, required := range []string{
		"func OneProxy_FlushDNS() *C.char {\n\treturn errStr(flushDNS((*proxy.DNSFlusher).FlushAll))\n}",
		"func OneProxy_Stop() *C.char {\n\tgMu.Lock()\n\tdefer gMu.Unlock()",
	} {
		if !strings.Contains(code, required) {
			t.Fatalf("missing manual error mapping or shared Stop mutex: %s", required)
		}
	}
}
