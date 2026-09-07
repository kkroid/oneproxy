package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
)

var (
	clashAPIBaseURL    = "http://127.0.0.1:9090"
	selectorHTTPClient = &http.Client{Timeout: time.Second}
)

type clashProxyState struct {
	Now string `json:"now"`
}

func selectorState(cfg *config.Config) (string, string) {
	return selectorStateFromAPI(selectorHTTPClient, clashAPIBaseURL, cfg)
}

func selectorStateFromAPI(client *http.Client, baseURL string, cfg *config.Config) (string, string) {
	selectorTag := cfg.Unified.Tag
	if selectorTag == "" {
		selectorTag = "proxy"
	}

	state, err := getClashProxyState(client, baseURL, selectorTag)
	if err != nil || state.Now == "" {
		return "", ""
	}

	mode := "manual"
	selectedTag := state.Now
	if state.Now == "auto" {
		mode = "auto"
		autoState, err := getClashProxyState(client, baseURL, "auto")
		if err != nil {
			return mode, ""
		}
		selectedTag = autoState.Now
	}

	for _, proxy := range cfg.GetEnabledProxies() {
		if "out-"+sanitizeTag(proxy.Name) == selectedTag {
			return mode, proxy.Name
		}
	}
	return mode, ""
}

func getClashProxyState(client *http.Client, baseURL, tag string) (clashProxyState, error) {
	response, err := client.Get(baseURL + "/proxies/" + url.PathEscape(tag))
	if err != nil {
		return clashProxyState{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return clashProxyState{}, fmt.Errorf("clash api returned %d", response.StatusCode)
	}

	var state clashProxyState
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		return clashProxyState{}, err
	}
	return state, nil
}

func selectionOutboundTag(name string, proxies []config.ProxyConfig) (string, error) {
	if name == "auto" {
		return "auto", nil
	}
	for _, proxy := range proxies {
		if proxy.Enabled && proxy.Name == name {
			return "out-" + sanitizeTag(name), nil
		}
	}
	return "", fmt.Errorf("proxy not found: %s", name)
}

func putSelectorSelection(client *http.Client, baseURL, selectorTag, outboundTag string) error {
	payload, err := json.Marshal(map[string]string{"name": outboundTag})
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPut,
		baseURL+"/proxies/"+url.PathEscape(selectorTag), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("clash api returned %d", response.StatusCode)
	}
	return nil
}

func sanitizeTag(name string) string {
	var tag strings.Builder
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9', character == '-', character == '_':
			tag.WriteRune(character)
		case character == ' ':
			tag.WriteByte('-')
		}
	}
	return tag.String()
}
