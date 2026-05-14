package ratelimit

import (
	"testing"
)

func TestAllowWithinBurst(t *testing.T) {
	il := New(1, 5) // 1 rps, burst=5
	for i := range 5 {
		if !il.Allow("127.0.0.1") {
			t.Fatalf("request %d should be allowed", i)
		}
	}
}

func TestExceedBurst(t *testing.T) {
	il := New(1, 3) // burst=3
	for range 3 {
		il.Allow("10.0.0.1")
	}
	if il.Allow("10.0.0.1") {
		t.Fatal("4th request should be denied after burst exhausted")
	}
}

func TestDifferentIPsIsolated(t *testing.T) {
	il := New(1, 1) // burst=1 per IP
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
