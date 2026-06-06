package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/client"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/stats"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/telemetry"
)

// TestPerfReport_5K drives N workers against an in-process mock orderbook for
// D seconds and writes a full performance report (CDF + percentiles + TPS)
// to PERF_REPORT_PATH. The brief's hard target is 5 000+ concurrent bots
// sustaining 10 000+ TPS; this harness produces the numbers that prove it.
//
//	go test -run TestPerfReport_5K ./...  -timeout 5m
//	$env:PERF_WORKERS=5000; $env:PERF_DURATION=60s; go test -run TestPerfReport_5K -v

func TestPerfReport_5K(t *testing.T) {
	if testing.Short() {
		t.Skip("perf report skipped under -short")
	}
	// CI saturates at the brief's full 5K×5 RPS = 25K aggregate load (GitHub
	// Actions runners are 2-core / 7GB). The benchmark is meant for manual
	// perf-report runs on a dev laptop, not a per-PR gate. Opt in explicitly
	// via PERF_BENCH=1; see docs/PERFORMANCE_REPORT.md for the canonical run.
	if os.Getenv("PERF_BENCH") != "1" {
		t.Skip("set PERF_BENCH=1 to run the 5K-bot perf harness (heavy)")
	}
	workers := envInt2(t, "PERF_WORKERS", 5000)
	dur := envDuration(t, "PERF_DURATION", 30*time.Second)
	rps := envFloat2(t, "PERF_RPS", 5.0)
	reportPath := os.Getenv("PERF_REPORT_PATH")
	if reportPath == "" {
		reportPath = filepath.Join(os.TempDir(), "iicpc-perf-report.json")
	}

	t.Logf("perf harness: %d workers · %s · %.1f rps each · target rps=%.0f", workers, dur, rps, float64(workers)*rps)

	// Minimal orderbook mock — returns a server-side ID + empty fills.
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

	// Capture EVERY latency into a single HDR histogram. This is the correct
	// way — merging happens at the bucket level, not by averaging per-window
	// P99s (which would understate tail latency).
	hist := hdrhistogram.New(1, 60_000_000_000, 3) // 1ns .. 60s, 0.1% precision
	var histMu sync.Mutex
	var errors atomic.Int64
	var requests atomic.Int64
	var zeroLat atomic.Int64

	recorder := &perfRecorder{hist: hist, mu: &histMu, errors: &errors, requests: &requests, zeroLat: &zeroLat}

	rec := stats.New() // unused but required by runWorker signature
	arrivals := gen.NewArrivals(gen.ArrivalPoisson, rps)
	// Tuned transport: production-realistic pool sized for K conns. Avoid
	// MaxConnsPerHost=workers (10K+) which blows Go's 10K-OS-thread limit on
	// Windows IOCP when dials block in syscalls. 512 idle keepalives is plenty
	// to amortize handshake cost while staying within OS limits.
	tr := &http.Transport{
		MaxIdleConns:        1024,
		MaxIdleConnsPerHost: 1024,
		MaxConnsPerHost:     1024,
		IdleConnTimeout:     90 * time.Second,
	}
	cli := &recordingClient{
		inner:    client.New(client.Config{BaseURL: srv.URL, Timeout: 5 * time.Second, Transport: tr}),
		recorder: recorder,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	start := time.Now()
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			g := gen.New(gen.Config{MidPrice: 100, PriceSigma: 1, CancelRatio: 0.3})
			runWorker(ctx, id, cli, g, rec, arrivals, telemetry.NewStub(), "perf", "sub-perf", fmt.Sprintf("bot-%d", id), log)
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	totalReq := requests.Load()
	totalErr := errors.Load()
	tps := float64(totalReq) / elapsed.Seconds()
	zeroCount := zeroLat.Load()
	t.Logf("done: %d requests in %s = %.0f req/s, %d errors, %d zero-lat (%.1f%%)", totalReq, elapsed.Round(time.Millisecond), tps, totalErr, zeroCount, float64(zeroCount)*100/float64(totalReq))
	t.Logf("percentiles ns: p50=%d p90=%d p95=%d p99=%d p999=%d p9999=%d max=%d",
		hist.ValueAtQuantile(50), hist.ValueAtQuantile(90), hist.ValueAtQuantile(95),
		hist.ValueAtQuantile(99), hist.ValueAtQuantile(99.9), hist.ValueAtQuantile(99.99),
		hist.Max())

	// Write the full report — CDF, percentiles, errors — to PERF_REPORT_PATH.
	report := perfReport{
		Workers:           workers,
		Duration:          elapsed,
		TotalRequests:     totalReq,
		TotalErrors:       totalErr,
		SubFloorLatencies: zeroCount,
		RequestsPerSec:    tps,
		TargetRPS:         float64(workers) * rps,
		Percentiles: map[string]int64{
			"p50":   hist.ValueAtQuantile(50),
			"p90":   hist.ValueAtQuantile(90),
			"p95":   hist.ValueAtQuantile(95),
			"p99":   hist.ValueAtQuantile(99),
			"p999":  hist.ValueAtQuantile(99.9),
			"p9999": hist.ValueAtQuantile(99.99),
			"max":   hist.Max(),
		},
		// 50-point CDF — every 2 percentile points.
		CDF: cdfPoints(hist),
	}
	if err := writeReport(reportPath, report); err != nil {
		t.Errorf("write report %s: %v", reportPath, err)
	} else {
		t.Logf("wrote report → %s", reportPath)
	}

	// Hard requirements: <0.1% error rate at scale. Zero is unrealistic on a
	// laptop where the OS occasionally hiccups; the brief cares about p99
	// stability, not zero failures.
	errPct := float64(totalErr) / float64(totalReq) * 100
	if errPct > 0.1 {
		t.Errorf("error rate %.3f%% > 0.1%% (errors=%d, total=%d)", errPct, totalErr, totalReq)
	}
	if workers >= 1000 && tps < 1000 {
		t.Logf("warning: sustained TPS %.0f below 1K with %d workers", tps, workers)
	}
}

// ---- helpers ----------------------------------------------------------------

type perfRecorder struct {
	hist     *hdrhistogram.Histogram
	mu       *sync.Mutex
	errors   *atomic.Int64
	requests *atomic.Int64
	zeroLat  *atomic.Int64 // count of zero-latency observations (clock-res artifacts)
}

// recordOK accepts a measured latency. Sub-floor measurements (latNs <= 0)
// happen on Windows when an HTTP roundtrip completes within one QPC tick
// (~100ns). They are *real* successes but we can't bucket their true latency,
// so we count them separately rather than poison the histogram with a synthetic
// 1ns value (which would systematically pull the median down and mislead).
func (r *perfRecorder) recordOK(latNs int64) {
	r.requests.Add(1)
	if latNs <= 0 {
		r.zeroLat.Add(1)
		return
	}
	r.mu.Lock()
	_ = r.hist.RecordValue(latNs)
	r.mu.Unlock()
}

func (r *perfRecorder) recordErr() {
	r.requests.Add(1)
	r.errors.Add(1)
}

// recordingClient wraps the REST client so we can capture latency for every
// request before delegating to the real implementation.
type recordingClient struct {
	inner    *client.REST
	recorder *perfRecorder
}

func (c *recordingClient) PlaceOrder(ctx context.Context, side string, price float64, qty int) (client.PlaceResult, error) {
	res, err := c.inner.PlaceOrder(ctx, side, price, qty)
	if err != nil {
		c.recorder.recordErr()
	} else {
		c.recorder.recordOK(res.LatencyNs)
	}
	return res, err
}

func (c *recordingClient) PlaceMarketOrder(ctx context.Context, side string, qty int) (client.PlaceResult, error) {
	res, err := c.inner.PlaceMarketOrder(ctx, side, qty)
	if err != nil {
		c.recorder.recordErr()
	} else {
		c.recorder.recordOK(res.LatencyNs)
	}
	return res, err
}

func (c *recordingClient) CancelOrder(ctx context.Context, id string) (int64, error) {
	lat, err := c.inner.CancelOrder(ctx, id)
	if err != nil {
		c.recorder.recordErr()
	} else {
		c.recorder.recordOK(lat)
	}
	return lat, err
}

func (c *recordingClient) Close() error { return c.inner.Close() }

type perfReport struct {
	Workers           int              `json:"workers"`
	Duration          time.Duration    `json:"duration_ns"`
	TotalRequests     int64            `json:"total_requests"`
	TotalErrors       int64            `json:"total_errors"`
	SubFloorLatencies int64            `json:"sub_floor_latencies"` // successes too fast to time on Windows QPC
	RequestsPerSec    float64          `json:"requests_per_sec"`
	TargetRPS         float64          `json:"target_rps"`
	Percentiles       map[string]int64 `json:"percentiles_ns"`
	CDF               []cdfPoint       `json:"cdf"`
}

type cdfPoint struct {
	Quantile float64 `json:"quantile"`
	ValueNs  int64   `json:"value_ns"`
}

func cdfPoints(h *hdrhistogram.Histogram) []cdfPoint {
	// 50 equally-spaced quantiles from 0.5 to 99.99
	out := make([]cdfPoint, 0, 50)
	for _, q := range []float64{
		0.5, 1, 2, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50,
		55, 60, 65, 70, 75, 80, 85, 90, 92, 94, 95, 96, 97, 98,
		98.5, 99, 99.2, 99.4, 99.5, 99.6, 99.7, 99.8, 99.9,
		99.95, 99.99,
	} {
		out = append(out, cdfPoint{Quantile: q, ValueNs: h.ValueAtQuantile(q)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Quantile < out[j].Quantile })
	return out
}

func writeReport(path string, r perfReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func envInt2(t *testing.T, key string, def int) int {
	t.Helper()
	if v := os.Getenv(key); v != "" {
		var n int
		_, err := fmt.Sscanf(v, "%d", &n)
		if err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envDuration(t *testing.T, key string, def time.Duration) time.Duration {
	t.Helper()
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

func envFloat2(t *testing.T, key string, def float64) float64 {
	t.Helper()
	if v := os.Getenv(key); v != "" {
		var f float64
		_, err := fmt.Sscanf(v, "%f", &f)
		if err == nil && f > 0 {
			return f
		}
	}
	return def
}
