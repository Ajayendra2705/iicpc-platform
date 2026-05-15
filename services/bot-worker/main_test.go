package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/stats"
)

func TestHealthz(t *testing.T) {
	rec := stats.New()
	srv := buildServer(":0", rec)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz: got %d want 200", w.Code)
	}
}

func TestMetrics(t *testing.T) {
	rec := stats.New()
	rec.RecordLatency(1_000_000) // 1ms
	rec.RecordLatency(2_000_000) // 2ms
	rec.RecordError()

	srv := buildServer(":0", rec)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("metrics: got %d want 200", w.Code)
	}
	var snap stats.Snapshot
	if err := json.NewDecoder(w.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Count != 3 {
		t.Errorf("count: got %d want 3", snap.Count)
	}
	if snap.Errors != 1 {
		t.Errorf("errors: got %d want 1", snap.Errors)
	}
	if snap.P50Ns <= 0 {
		t.Error("P50 must be positive")
	}
}

func TestMetricsContentType(t *testing.T) {
	rec := stats.New()
	srv := buildServer(":0", rec)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Handler.ServeHTTP(w, r)
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q want application/json", ct)
	}
}
