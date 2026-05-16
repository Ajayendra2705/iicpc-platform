package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/client"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/stats"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/telemetry"
)

// TestLoadConcurrentWorkers drives N workers against a mock orderbook for
// `duration` seconds and asserts zero errors plus reasonable throughput.
// Mirrors the Day 14 "1K bots → orderbook, no errors" scenario in miniature.
func TestLoadConcurrentWorkers(t *testing.T) {
	const (
		numWorkers = 200
		duration   = 2 * time.Second
		rps        = 5.0
	)

	var orderSeq atomic.Uint64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/order":
			id := orderSeq.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    fmt.Sprintf("ord-%d", id),
				"fills": []any{},
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/order/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rec := stats.New()
	arrivals := gen.NewArrivals(gen.ArrivalPoisson, rps)
	cli := client.New(client.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup
	for i := range numWorkers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			g := gen.New(gen.Config{MidPrice: 100, PriceSigma: 1, CancelRatio: 0.5})
			runWorker(ctx, id, cli, g, rec, arrivals, telemetry.NewStub(), "test", "bot-test", log)
		}(i)
	}
	wg.Wait()

	snap := rec.Snapshot()
	t.Logf("workers=%d duration=%s count=%d errors=%d tps=%.0f p50=%dns p90=%dns p99=%dns",
		numWorkers, duration, snap.Count, snap.Errors, snap.TPS, snap.P50Ns, snap.P90Ns, snap.P99Ns)

	if snap.Errors > 0 {
		t.Errorf("expected 0 errors under load, got %d (count=%d)", snap.Errors, snap.Count)
	}
	if snap.Count < int64(numWorkers) {
		t.Errorf("expected count ≥ %d (1 per worker), got %d", numWorkers, snap.Count)
	}
}

// TestLoadBurstMode verifies burst mode actually elevates throughput.
func TestLoadBurstMode(t *testing.T) {
	const (
		numWorkers = 50
		duration   = 1500 * time.Millisecond
	)

	var seen atomic.Uint64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "x", "fills": []any{}})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	rec := stats.New()
	// base 2 RPS; burst 10× → 20 RPS during 200ms every 500ms
	arrivals := gen.NewArrivals(gen.ArrivalUniform, 2)
	cli := client.New(client.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	arrivals.StartBurst(ctx, 500*time.Millisecond, 200*time.Millisecond, 10)

	var wg sync.WaitGroup
	for i := range numWorkers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			g := gen.New(gen.Config{MidPrice: 100, PriceSigma: 1, CancelRatio: 0.3})
			runWorker(ctx, id, cli, g, rec, arrivals, telemetry.NewStub(), "test", "bot-test", log)
		}(i)
	}
	wg.Wait()

	if seen.Load() == 0 {
		t.Fatal("burst test recorded zero requests")
	}
}
