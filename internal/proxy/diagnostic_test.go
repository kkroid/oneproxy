package proxy

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
	"github.com/kkroid/oneproxy/internal/logger"
)

func TestHealthProbeObservedStages(t *testing.T) {
	for _, tc := range []struct{ url, stage string }{
		{"http://192.0.2.1/", "response_headers"},
		{"https://192.0.2.1/", "target_tls_handshake"},
	} {
		t.Run(tc.stage, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				greeting := make([]byte, 3)
				if _, err := io.ReadFull(conn, greeting); err != nil {
					return
				}
				_, _ = conn.Write([]byte{5, 0})
				request := make([]byte, 10) // CONNECT to the fixture's IPv4 address.
				if _, err := io.ReadFull(conn, request); err != nil {
					return
				}
				_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 1})
				_, _ = io.Copy(io.Discard, conn) // Stall after SOCKS success; no external dial.
			}()
			path := filepath.Join(t.TempDir(), "app.log")
			log, err := logger.New(path, 0, logger.DefaultBackups)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			hc := NewHealthChecker(&config.Config{HealthCheck: config.HealthCheck{TimeoutSeconds: 1, TestURL: tc.url}}, nil)
			hc.SetLogger(log)
			result := hc.CheckProxy("fixture", listener.Addr().(*net.TCPAddr).Port)
			if result.IsHealthy {
				t.Fatal("stalled fixture reported healthy")
			}
			data, err := os.ReadFile(logger.DatePath(path, time.Now()))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "stage="+tc.stage) || !strings.Contains(string(data), "dial_ip=unknown") {
				t.Fatalf("wrong observation: %s", data)
			}
			<-done
		})
	}
}

func TestDiagnosticFieldsDoNotInventStagesOrCopySecrets(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{"ERROR [123 5.2s] connection: outbound/vless[secret-node]: dial tcp 192.0.2.1:443: i/o timeout password=secret", "dial_ip=192.0.2.1 stage=tcp_connect elapsed=5.2s"},
		{"ERROR connection: outbound/vless[secret-node]: dial tcp [2001:db8::1]:443: timeout", "dial_ip=2001:db8::1 stage=tcp_connect elapsed=unknown"},
		{"ERROR connection: outbound/vless[secret-node]: EOF https://secret:secret@secret.invalid", "dial_ip=unknown stage=unknown elapsed=unknown"},
		{"ERROR connection: lookup secret.invalid: timeout", "dial_ip=unknown stage=dns elapsed=unknown"},
		{"ERROR connection: dial tcp 999.999.999.999:443: timeout", "dial_ip=unknown stage=unknown elapsed=unknown"},
	} {
		got := diagnosticFields([]byte(tc.line))
		if !strings.Contains(got, tc.want) || strings.Contains(got, "secret") {
			t.Errorf("diagnostic = %q, want %q without secrets", got, tc.want)
		}
	}
}

func TestHealthDiagnosticsInitialFailureFlappingAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	log, err := logger.New(path, 0, logger.DefaultBackups)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	hc := NewHealthChecker(&config.Config{}, nil)
	hc.SetLogger(log)
	hc.results["secret-node"] = &HealthResult{LocalPort: 10801}
	for i := 0; i < 5; i++ {
		hc.updateResult("secret-node", false, 5*time.Second, "Get https://secret:secret@secret.invalid/token: context deadline exceeded")
		hc.updateResult("secret-node", true, time.Millisecond, "")
	}
	data, err := os.ReadFile(logger.DatePath(path, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "secret") || strings.Count(text, "health failed") != 1 || strings.Count(text, "health recovered") != 1 {
		t.Fatalf("unsafe or unlimited diagnostics: %s", text)
	}
	for _, field := range []string{"local_port=10801", "stage=unknown", "dial_ip=unknown", "kind=timeout", "elapsed=5s", "failure_duration=5s"} {
		if !strings.Contains(text, field) {
			t.Errorf("missing %s: %s", field, text)
		}
	}
	if !hc.results["secret-node"].failureSince.IsZero() {
		t.Fatal("recovery did not clear outage")
	}
	hc.results["secret-node"].lastFailureLog = time.Now().Add(-time.Minute)
	hc.updateResult("secret-node", false, time.Second, "HTTP 503")
	data, _ = os.ReadFile(logger.DatePath(path, time.Now()))
	if strings.Count(string(data), "health failed") != 2 || !strings.Contains(string(data), "kind=http_status") {
		t.Fatal("diagnostic did not resume after rate limit")
	}
}
