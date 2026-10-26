package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkroid/oneproxy/internal/logger"
)

type failingLog struct {
	failed bool
	bytes.Buffer
}

func (f *failingLog) Write(data []byte) (int, error) {
	if !f.failed {
		f.failed = true
		return 0, errors.New("opaque-secret-value")
	}
	return f.Buffer.Write(data)
}

func TestProcessOutputDrainsFailureAndPreservesPrivateTail(t *testing.T) {
	file := &failingLog{}
	p := &processOutput{file: file}
	first := []byte(strings.Repeat("x", 5000) + "upstream rejected: opaque-secret-value\n")
	if n, err := p.Write(first); n != len(first) || err != nil {
		t.Fatalf("output stopped draining: %d, %v", n, err)
	}
	if p.lost != uint64(len(first)) || len(p.tail) != 4096 {
		t.Fatal("missing loss accounting or bounded tail")
	}
	if _, err := p.Write([]byte("final private tail\n")); err != nil {
		t.Fatal(err)
	}
	cmd := &exec.Cmd{Stderr: p}
	err := processExitError(cmd, errors.New("exit status 26"))
	if strings.Contains(err.Error(), "opaque-secret-value") || !strings.Contains(err.Error(), "GetLogs") {
		t.Fatalf("unsafe/unhelpful public error: %v", err)
	}
	m := &Manager{lastOutput: p}
	lines, err := m.GetLogs(2)
	if err != nil || len(lines) != 2 || !strings.Contains(lines[0], "opaque-secret-value") || lines[1] != "final private tail" {
		t.Fatalf("private tail unavailable: %q, %v", lines, err)
	}
	if got, err := m.GetLogs(0); err != nil || len(got) != 0 {
		t.Fatalf("GetLogs(0) = %q, %v", got, err)
	}
}

func TestCoreDiagnosticFormatsAndPrivacy(t *testing.T) {
	// DNS fixtures come from isolated sing-box 1.13.14 + local TCP DNS probes.
	cases := []struct{ line, kind string }{
		{"ERROR connection: open connection to target.invalid:443 using outbound/vless[tag]: lookup proxy.invalid: SERVFAIL", "dns_resolution_failed"},
		{"ERROR connection: lookup proxy.invalid: NXDOMAIN", "dns_resolution_failed"},
		{"ERROR dns: exchange failed for proxy.invalid. IN A: i/o timeout", "dns_resolution_failed"},
		{"ERROR connection: lookup proxy.invalid: i/o timeout", "dns_resolution_failed"},
		{"ERROR connection: dial tcp 192.0.2.1:443: i/o timeout", "connection_timeout"},
		{"ERROR connection: dial tcp 192.0.2.1:443: connectex: connection timed out", "connection_timeout"},
		{"INFO dns: exchanged proxy.invalid NOERROR", ""},
		{"INFO connection established", ""},
	}
	for _, tc := range cases {
		if got := coreFailureKind([]byte(tc.line)); got != tc.kind {
			t.Errorf("kind(%q) = %q, want %q", tc.line, got, tc.kind)
		}
	}
	for _, line := range []string{
		"listen failed: address already in use",
		"invalid generated configuration",
		"parse config: unknown field",
		"permission denied",
		"open certificate: no such file",
		"invalid public key",
		"upstream rejected",
	} {
		secret := " https://user:password@example.invalid/token-secret?uuid=1234&public_key=reality-secret opaque-secret-value"
		got := safeCoreDiagnostic([]byte(line + secret))
		for _, sensitive := range []string{"password", "token-secret", "uuid=", "reality-secret", "opaque-secret-value", "example.invalid"} {
			if strings.Contains(got, sensitive) {
				t.Errorf("diagnostic leaked %q: %q", sensitive, got)
			}
		}
	}
}

func TestProcessOutputSplitAndConcurrentDiagnosticsAreSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	app, err := logger.New(path, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	p := &processOutput{file: io.Discard, appLog: app}
	for _, fragment := range []string{"ERROR connection: look", "up proxy.invalid: SERV", "FAIL password=opaque-secret-value\n"} {
		_, _ = p.Write([]byte(fragment))
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = p.Write([]byte("ERROR connection: dial tcp 192.0.2.1:443: i/o timeout uuid=opaque-secret-value\n"))
		}()
	}
	wg.Wait()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "dns_resolution_failed") != 1 || strings.Count(string(got), "connection_timeout") != 1 {
		t.Fatalf("missing/rate-unlimited classification: %s", got)
	}
	if strings.Contains(string(got), "opaque-secret-value") || strings.Contains(string(got), "192.0.2.1") || strings.Contains(string(got), "proxy.invalid") {
		t.Fatalf("upstream leaked: %s", got)
	}
}

func TestManagerUnknownStartupErrorRetainsPrivateTail(t *testing.T) {
	m, _ := newTestManager(t, false, "secret-exit")
	// This tests diagnostic handling, not the scheduling latency of a test EXE.
	m.startupWindow = 5 * time.Second
	err := m.Start()
	if err == nil || strings.Contains(err.Error(), "opaque-secret-value") || !strings.Contains(err.Error(), "GetLogs") {
		t.Fatalf("Start error = %v", err)
	}
	lines, readErr := m.GetLogs(10)
	if readErr != nil || !strings.Contains(strings.Join(lines, "\n"), "opaque-secret-value") {
		t.Fatalf("private startup tail = %q, %v", lines, readErr)
	}
}

func TestManagerCheckUnknownErrorRetainsPrivateTail(t *testing.T) {
	m, _ := newTestManager(t, false, "secret-exit")
	err := m.Check()
	if err == nil || strings.Contains(err.Error(), "opaque-secret-value") || !strings.Contains(err.Error(), "GetLogs") {
		t.Fatalf("Check error = %v", err)
	}
	lines, readErr := m.GetLogs(10)
	if readErr != nil || !strings.Contains(strings.Join(lines, "\n"), "opaque-secret-value") {
		t.Fatalf("private check tail = %q, %v", lines, readErr)
	}
}

func waitLogCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("log condition not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestManagerConcurrentCoreStreamsRotateAcrossRestart(t *testing.T) {
	m, _ := newTestManager(t, false, "output-burst")
	m.startupWindow = 20 * time.Millisecond
	t.Cleanup(func() { _ = m.Stop() })
	donePath := filepath.Join(filepath.Dir(m.configPath), "output.done")
	t.Setenv("ONEPROXY_MANAGER_OUTPUT_DONE", donePath)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		waitLogCondition(t, func() bool { _, err := os.Stat(donePath); return err == nil })
		if !m.IsRunning() {
			t.Fatal("output or rotation stopped core")
		}
		if err := m.Stop(); err != nil {
			t.Fatal(err)
		}
		for i := 0; i <= 3; i++ {
			name := m.logPath
			if i > 0 {
				name = fmt.Sprintf("%s.%d", name, i)
			}
			info, err := os.Stat(name)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > logger.DefaultMaxSize {
				t.Fatalf("core log exceeded bound: %d", info.Size())
			}
		}
		if _, err := os.Stat(m.logPath + ".1"); err != nil {
			t.Fatal("core output did not rotate")
		}
		lines, err := m.GetLogs(1)
		if err != nil || len(lines) != 1 || lines[0] != "output-burst-complete" {
			t.Fatalf("tail = %q, %v", lines, err)
		}
		if attempt == 0 {
			if err := os.Remove(donePath); err != nil {
				t.Fatal(err)
			}
			if err := m.Start(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestManagerBothLogsBlockedRecoverWithoutStoppingCore(t *testing.T) {
	m, _ := newTestManager(t, false, "output-stream")
	m.startupWindow = 20 * time.Millisecond
	t.Cleanup(func() { _ = m.Stop() })
	appPath := filepath.Join(filepath.Dir(m.logPath), "app.log")
	for _, path := range []string{m.logPath, appPath} {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		// Temporary sparse/zero-filled file: never touch installed logs.
		if err := file.Truncate(logger.DefaultMaxSize + 1); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		if err := os.WriteFile(path+".oversized", []byte("earlier evidence"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	app, err := logger.New(appPath, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	m.SetLogger(app)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	app.Info("blocked application log")
	waitLogCondition(t, func() bool {
		m.lastOutput.mu.Lock()
		defer m.lastOutput.mu.Unlock()
		return m.lastOutput.lost > 100
	})
	if !m.IsRunning() {
		t.Fatal("logging conflict stopped core")
	}
	for _, path := range []string{m.logPath, appPath} {
		info, err := os.Stat(path)
		if err != nil || info.Size() != logger.DefaultMaxSize+1 {
			t.Fatal("conflict changed/grows evidence")
		}
		if err := os.Rename(path+".oversized", path+".manual-archive"); err != nil {
			t.Fatal(err)
		}
	}
	waitLogCondition(t, func() bool {
		app.Info("application recovery probe")
		data, _ := os.ReadFile(appPath)
		core, _ := os.ReadFile(m.logPath)
		return bytes.Contains(data, []byte("bytes lost")) && bytes.Contains(core, []byte("bytes lost"))
	})
	if !m.IsRunning() {
		t.Fatal("recovery stopped core")
	}
	if err := m.Restart(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{m.logPath, appPath} {
		info, err := os.Stat(path + ".oversized")
		if err != nil || info.Size() != logger.DefaultMaxSize+1 {
			t.Fatal("old active file not preserved")
		}
		data, _ := os.ReadFile(path + ".manual-archive")
		if string(data) != "earlier evidence" {
			t.Fatal("earlier archive changed")
		}
	}
}
