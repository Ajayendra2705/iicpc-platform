package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDGeneratesWhenAbsent(t *testing.T) {
	var seenByHandler string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenByHandler = w.Header().Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	got := rec.Header().Get("X-Request-ID")
	if got == "" {
		t.Fatal("expected a generated X-Request-ID in the response")
	}
	if len(got) != 16 { // 8 random bytes hex-encoded
		t.Fatalf("expected 16-char hex request id, got %q (len %d)", got, len(got))
	}
	if seenByHandler != got {
		t.Fatal("response header and handler-visible header should match")
	}
}

func TestRequestIDPreservesIncoming(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "trace-abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "trace-abc" {
		t.Fatalf("expected incoming request id to be preserved, got %q", got)
	}
}

func TestAccessLogPassesThroughAndCapturesStatus(t *testing.T) {
	called := false
	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if !called {
		t.Fatal("AccessLog must invoke the next handler")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d to pass through, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestStatusRecorderDefaultsTo200(t *testing.T) {
	// A handler that writes a body without an explicit WriteHeader implies 200.
	var captured int
	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec, ok := w.(*statusRecorder)
		if !ok {
			t.Fatalf("expected wrapped writer to be *statusRecorder, got %T", w)
		}
		captured = rec.status
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if captured != http.StatusOK {
		t.Fatalf("expected statusRecorder to default to 200, got %d", captured)
	}
}
