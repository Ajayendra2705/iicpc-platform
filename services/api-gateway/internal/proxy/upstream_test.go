package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRejectsInvalidTarget(t *testing.T) {
	if _, err := New("://not a url"); err == nil {
		t.Fatal("expected error for malformed target URL, got nil")
	}
}

func TestNewProxiesToUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "from upstream "+r.URL.Path)
	}))
	defer upstream.Close()

	h, err := New(upstream.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	front := httptest.NewServer(h)
	defer front.Close()

	resp, err := http.Get(front.URL + "/submissions")
	if err != nil {
		t.Fatalf("GET through proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("expected upstream status %d to pass through, got %d", http.StatusTeapot, resp.StatusCode)
	}
	if resp.Header.Get("X-Upstream") != "yes" {
		t.Fatal("expected upstream response header to pass through")
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "from upstream /submissions" {
		t.Fatalf("unexpected proxied body: %q", body)
	}
}
