//go:build windows

package proxy

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

type fakeDNSFlushProc struct {
	findErr error
	result  uintptr
	callErr error
	calls   int
}

func (p *fakeDNSFlushProc) Find() error { return p.findErr }
func (p *fakeDNSFlushProc) Call(...uintptr) (uintptr, uintptr, error) {
	p.calls++
	return p.result, 0, p.callErr
}

func TestWindowsDNSFlushAPI(t *testing.T) {
	missing := errors.New("procedure unavailable")
	for _, tt := range []struct {
		name string
		proc fakeDNSFlushProc
		want error
		text string
	}{
		{"success", fakeDNSFlushProc{result: 1}, nil, ""},
		{"success ignores stale last error", fakeDNSFlushProc{result: 1, callErr: syscall.ERROR_ACCESS_DENIED}, nil, ""},
		{"missing API", fakeDNSFlushProc{findErr: missing}, missing, "load DnsFlushResolverCache"},
		{"API failure", fakeDNSFlushProc{callErr: syscall.ERROR_ACCESS_DENIED}, syscall.ERROR_ACCESS_DENIED, "DnsFlushResolverCache failed"},
		{"no error code", fakeDNSFlushProc{callErr: syscall.Errno(0)}, nil, "returned FALSE"},
		{"nil error code", fakeDNSFlushProc{}, nil, "returned FALSE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := flushWindowsDNS(&tt.proc)
			if tt.text == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.text) {
				t.Fatalf("error = %v, want %q", err, tt.text)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want wrapped %v", err, tt.want)
			}
			wantCalls := 1
			if tt.proc.findErr != nil {
				wantCalls = 0
			}
			if tt.proc.calls != wantCalls {
				t.Fatalf("API calls = %d, want %d", tt.proc.calls, wantCalls)
			}
		})
	}
}
