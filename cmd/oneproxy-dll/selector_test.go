package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kkroid/oneproxy/internal/config"
)

func TestSelectionOutboundTag(t *testing.T) {
	proxies := []config.ProxyConfig{
		{Name: "Node One", Enabled: true},
		{Name: "Disabled", Enabled: false},
	}

	for _, test := range []struct {
		name string
		want string
	}{
		{name: "auto", want: "auto"},
		{name: "Node One", want: "out-Node-One"},
	} {
		got, err := selectionOutboundTag(test.name, proxies)
		if err != nil || got != test.want {
			t.Fatalf("selectionOutboundTag(%q) = %q, %v; want %q", test.name, got, err, test.want)
		}
	}

	if _, err := selectionOutboundTag("Disabled", proxies); err == nil {
		t.Fatal("selectionOutboundTag() accepted a disabled proxy")
	}
}

func TestPutSelectorSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.EscapedPath() != "/proxies/custom%20selector" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.EscapedPath())
		}
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["name"] != "out-Node-One" {
			t.Errorf("payload name = %q", payload["name"])
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := putSelectorSelection(server.Client(), server.URL, "custom selector", "out-Node-One"); err != nil {
		t.Fatal(err)
	}
}

func TestSelectorState(t *testing.T) {
	selectorNow := "auto"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		now := "out-Node-Two"
		if request.URL.Path == "/proxies/proxy" {
			now = selectorNow
		}
		_ = json.NewEncoder(response).Encode(map[string]string{"now": now})
	}))
	defer server.Close()

	cfg := &config.Config{Unified: config.UnifiedConfig{Port: 1082}, Proxies: []config.ProxyConfig{
		{Name: "Node One", Enabled: true},
		{Name: "Node Two", Enabled: true},
	}}

	mode, selected := selectorStateFromAPI(server.Client(), server.URL, cfg)
	if mode != "auto" || selected != "Node Two" {
		t.Fatalf("selectorState() = %q, %q; want auto, Node Two", mode, selected)
	}

	selectorNow = "out-Node-One"
	mode, selected = selectorStateFromAPI(server.Client(), server.URL, cfg)
	if mode != "manual" || selected != "Node One" {
		t.Fatalf("selectorState() = %q, %q; want manual, Node One", mode, selected)
	}
}
