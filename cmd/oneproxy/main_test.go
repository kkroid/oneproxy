package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantConfig string
		wantHelp   bool
		wantErr    bool
	}{
		{name: "config", args: []string{"--config", "config.json"}, wantConfig: "config.json"},
		{name: "config equals", args: []string{"--config=config.json"}, wantConfig: "config.json"},
		{name: "help", args: []string{"--help"}, wantHelp: true},
		{name: "missing flag", wantErr: true},
		{name: "missing value", args: []string{"--config"}, wantErr: true},
		{name: "unknown flag", args: []string{"--unknown"}, wantErr: true},
		{name: "positional", args: []string{"start"}, wantErr: true},
		{name: "config and positional", args: []string{"--config", "config.json", "start"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got, err := parseArgs(test.args, &stderr)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseArgs() error = %v, wantErr %v", err, test.wantErr)
			}
			if got.configPath != test.wantConfig || got.help != test.wantHelp {
				t.Fatalf("parseArgs() = %+v, want config=%q help=%v", got, test.wantConfig, test.wantHelp)
			}
		})
	}
}

func TestHelpDoesNotRequireRuntimeFiles(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--help) = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "oneproxy --config <path>") {
		t.Fatalf("help output = %q", stdout.String())
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{nil, {"--config"}, {"--bad"}, {"start"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%q) = %d, want 2; stderr=%q", args, code, stderr.String())
		}
		if !strings.Contains(stderr.String(), usageText) {
			t.Fatalf("run(%q) stderr lacks usage: %q", args, stderr.String())
		}
	}
}

func TestResolveRuntimePaths(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "invocation")
	home := filepath.Join(t.TempDir(), "home")
	executable := filepath.Join(t.TempDir(), "install", "oneproxy")

	paths, err := resolveRuntimePaths(filepath.Join("configs", "user.json"), cwd, home, executable)
	if err != nil {
		t.Fatal(err)
	}
	if paths.configPath != filepath.Join(cwd, "configs", "user.json") {
		t.Errorf("configPath = %q", paths.configPath)
	}
	if paths.singboxPath != filepath.Join(filepath.Dir(executable), "bin", singBoxExecutableName()) {
		t.Errorf("singboxPath = %q", paths.singboxPath)
	}
	if paths.generatedConfig != filepath.Join(home, ".oneproxy", singboxConfigFile) {
		t.Errorf("generatedConfig = %q", paths.generatedConfig)
	}
	if paths.logPath != filepath.Join(home, ".oneproxy", "logs", "singbox.log") {
		t.Errorf("logPath = %q", paths.logPath)
	}
}
