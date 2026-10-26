package config

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Opt-in: the provided core runs only a temporary generated config and loopback
// services. No installed configuration, real subscription, or live core is used.
func TestSingBoxDomainDNSCacheAndFailure(t *testing.T) {
	core := os.Getenv("ONEPROXY_TEST_SINGBOX")
	if core == "" {
		t.Skip("set ONEPROXY_TEST_SINGBOX to a sing-box 1.13 executable")
	}
	dnsListener := localListener(t)
	endpoint := localListener(t)
	var queries, connections, mode atomic.Int32
	go func() {
		for {
			conn, err := endpoint.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			conn.Close()
		}
	}()
	go func() {
		for {
			conn, err := dnsListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
				for {
					var size uint16
					if binary.Read(conn, binary.BigEndian, &size) != nil {
						return
					}
					packet := make([]byte, size)
					if _, err := io.ReadFull(conn, packet); err != nil {
						return
					}
					var request dnsmessage.Message
					if request.Unpack(packet) != nil || len(request.Questions) != 1 {
						return
					}
					queries.Add(1)
					response := dnsmessage.Message{Header: dnsmessage.Header{ID: request.ID, Response: true, RecursionAvailable: true}, Questions: request.Questions}
					q := request.Questions[0]
					switch mode.Load() {
					case 1:
						response.Header.RCode = dnsmessage.RCodeServerFailure
					case 2:
						response.Header.RCode = dnsmessage.RCodeNameError
						response.Authorities = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeSOA, Class: dnsmessage.ClassINET, TTL: 2}, Body: &dnsmessage.SOAResource{NS: q.Name, MBox: q.Name, MinTTL: 2}}}
					default:
						if q.Type == dnsmessage.TypeA {
							response.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 2}, Body: &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}}}}
						}
					}
					data, _ := response.Pack()
					_ = binary.Write(conn, binary.BigEndian, uint16(len(data)))
					if _, err := conn.Write(data); err != nil {
						return
					}
				}
			}()
		}
	}()

	reservation := localListener(t)
	inboundPort := reservation.Addr().(*net.TCPAddr).Port
	reservation.Close()
	px, err := ParseSubscriptionLine(testVLESSURI)
	if err != nil {
		t.Fatal(err)
	}
	px.Name, px.Server, px.Port = "fixture", "dns-fixture.test", endpoint.Addr().(*net.TCPAddr).Port
	px.RealityPublicKey = "CQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // synthetic X25519 base point
	px.LocalPort, px.Enabled = inboundPort, true
	cfg := &Config{RouteMode: "direct", LogLevel: "debug", Inbound: InboundConfig{Listen: "127.0.0.1"}, Proxies: []ProxyConfig{px}}
	generated, err := NewSingBoxGenerator(cfg, t.TempDir()).Generate()
	if err != nil {
		t.Fatal(err)
	}
	// Only the resolver address changes; cache defaults and VLESS dial settings
	// are exactly those shipped by the generator.
	generated.DNS.Servers[0].(map[string]interface{})["address"] = "tcp://" + dnsListener.Addr().String()
	dir := t.TempDir()
	path := filepath.Join(dir, "core.json")
	data, _ := json.Marshal(generated)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "core.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(core, "run", "-c", path)
	cmd.Env = append(os.Environ(), "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true")
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = log.Close()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Log(string(data)) // synthetic config only
		}
	})
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(inboundPort))
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("isolated core did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	request := func() {
		t.Helper()
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = io.WriteString(conn, "CONNECT 192.0.2.1:80 HTTP/1.1\r\nHost: 192.0.2.1:80\r\n\r\nx")
		_, _ = io.Copy(io.Discard, conn)
	}
	request()
	if queries.Load() != 1 || connections.Load() != 1 {
		t.Fatalf("domain did not use configured DNS: queries=%d dials=%d", queries.Load(), connections.Load())
	}
	request()
	if queries.Load() != 1 || connections.Load() != 2 {
		t.Fatal("positive TTL cache not used")
	}
	time.Sleep(2200 * time.Millisecond)
	mode.Store(1)
	request()
	request()
	if queries.Load() != 3 || connections.Load() != 2 {
		t.Fatalf("expired answer reused or SERVFAIL cached: queries=%d dials=%d", queries.Load(), connections.Load())
	}
	mode.Store(2)
	request()
	request()
	if queries.Load() != 4 || connections.Load() != 2 {
		t.Fatalf("NXDOMAIN TTL cache failed: queries=%d dials=%d", queries.Load(), connections.Load())
	}
	time.Sleep(2200 * time.Millisecond)
	mode.Store(0)
	request()
	if queries.Load() != 5 || connections.Load() != 3 {
		t.Fatalf("did not recover after negative TTL: queries=%d dials=%d", queries.Load(), connections.Load())
	}
	_ = log.Sync()
	output, _ := os.ReadFile(logPath)
	if !strings.Contains(string(output), "lookup dns-fixture.test: SERVFAIL") || !strings.Contains(string(output), "lookup dns-fixture.test: NXDOMAIN") {
		t.Fatal("core did not identify DNS lookup failure")
	}
	t.Logf("TTL=2s: queries=%d endpoint dials=%d; cache hit, expiry, uncached SERVFAIL, cached NXDOMAIN and recovery verified", queries.Load(), connections.Load())
}

func localListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}
