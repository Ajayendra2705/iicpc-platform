package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPDirectUsesRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.5:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4") // ignored when trustedHops=0
	if got := clientIP(r, 0); got != "203.0.113.5" {
		t.Errorf("trustedHops=0: got %q want 203.0.113.5 (RemoteAddr)", got)
	}
}

func TestClientIPOneTrustedProxy(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:80" // the ingress
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := clientIP(r, 1); got != "198.51.100.7" {
		t.Errorf("trustedHops=1: got %q want 198.51.100.7 (real client)", got)
	}
}

func TestClientIPIgnoresSpoofedXFF(t *testing.T) {
	// Client forges an XFF entry; the trusted ingress appends the real client IP,
	// so the rightmost (trustedHops=1) entry is the real one, not the spoof.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:80"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 198.51.100.7")
	if got := clientIP(r, 1); got != "198.51.100.7" {
		t.Errorf("spoof: got %q want 198.51.100.7 (spoofed 6.6.6.6 must be ignored)", got)
	}
}

func TestClientIPFallsBackWhenXFFShort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:80"
	// trustedHops=2 but only one XFF entry → fall back to RemoteAddr.
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := clientIP(r, 2); got != "10.0.0.1" {
		t.Errorf("short XFF: got %q want 10.0.0.1 (RemoteAddr fallback)", got)
	}
}

func TestAllowWithinBurst(t *testing.T) {
	il := New(1, 5, 0) // 1 rps, burst=5
	for i := range 5 {
		if !il.Allow("127.0.0.1") {
			t.Fatalf("request %d should be allowed", i)
		}
	}
}

func TestExceedBurst(t *testing.T) {
	il := New(1, 3, 0) // burst=3
	for range 3 {
		il.Allow("10.0.0.1")
	}
	if il.Allow("10.0.0.1") {
		t.Fatal("4th request should be denied after burst exhausted")
	}
}

func TestDifferentIPsIsolated(t *testing.T) {
	il := New(1, 1, 0) // burst=1 per IP
	if !il.Allow("1.1.1.1") {
		t.Fatal("first IP first request should be allowed")
	}
	if !il.Allow("2.2.2.2") {
		t.Fatal("second IP first request should be allowed (independent bucket)")
	}
	if il.Allow("1.1.1.1") {
		t.Fatal("first IP second request should be denied")
	}
}
