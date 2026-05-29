package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/client"
)

// TestSaturationCurve ramps aggregate RPS in steps against an in-process mock
// orderbook and records the step at which the bot fleet first crosses a
// failure threshold. This is the PS deliverable §"Throughput: Maximum
// transactions per second (TPS) handled before failure" — a single steady-
// state number doesn't show *where* the system breaks, only that it sustains
// some load. The saturation curve does.
//
// Output: PERF_SATURATION_PATH (default $TMP/iicpc-saturation.json) gets a
// list of step records: {aggregate_rps_target, achieved_rps, errors,
// error_rate, p99_ns}. The first step with error_rate > FailureThreshold
// (default 1%) is the "max TPS before failure" answer.
//
//	$env:PERF_SATURATION=1; go test -run TestSaturationCurve -timeout 5m
//
// Heavy: 7 steps × 5s each = ~35s on a dev laptop. Gated behind
// PERF_SATURATION=1 so CI never runs it.
func TestSaturationCurve(t *testing.T) {
	if testing.Short() {
		t.Skip("saturation test skipped under -short")
	}
	if os.Getenv("PERF_SATURATION") != "1" {
		t.Skip("set PERF_SATURATION=1 to run the saturation/ramp test")
	}

	workers := envInt2(t, "SAT_WORKERS", 500)
	stepDur := envDuration(t, "SAT_STEP_DURATION", 5*time.Second)
	failThreshold := envFloat2(t, "SAT_FAIL_THRESHOLD", 0.01) // 1% error rate
	reportPath := os.Getenv("PERF_SATURATION_PATH")
	if reportPath == "" {
		reportPath = filepath.Join(os.TempDir(), "iicpc-saturation.json")
	}

	// Per-worker RPS at each step. Aggregate = workers × per-worker RPS.
	steps := []float64{2, 4, 8, 12, 16, 20, 30}

	// Mock orderbook with a simulated processing cost. Constant 50µs sleep
	// approximates a real matching engine's per-order CPU. This is the lever
	// that creates a saturation point: once aggregate RPS exceeds what the
	// mock can serve, error rate and tail latency climb sharply.
	var orderSeq atomic.Uint64
	procTime := envDuration(t, "SAT_PROC_TIME", 50*time.Microsecond)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if procTime > 0 {
			time.Sleep(procTime)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/order":
			id := orderSeq.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("ord-%d", id)})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/order/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	type stepReport struct {
		TargetAggregateRPS float64 `json:"target_aggregate_rps"`
		AchievedRPS        float64 `json:"achieved_rps"`
		Requests           int64   `json:"requests"`
		Errors             int64   `json:"errors"`
		ErrorRate          float64 `json:"error_rate"`
		P50Ns              int64   `json:"p50_ns"`
		P99Ns              int64   `json:"p99_ns"`
	}
	type report struct {
		WorkerCount    int          `json:"workers"`
		StepDurationNs int64        `json:"step_duration_ns"`
		ProcTimeNs     int64        `json:"server_proc_time_ns"`
		FailThreshold  float64      `json:"failure_threshold"`
		BreakpointStep int          `json:"breakpoint_step"` // index into Steps; -1 if no break
		BreakpointRPS  float64      `json:"breakpoint_rps"`  // achieved RPS at breakpoint
		LastGoodRPS    float64      `json:"last_good_rps"`   // last step where error_rate ≤ threshold
		Steps          []stepReport `json:"steps"`
	}

	r := report{
		WorkerCount:    workers,
		StepDurationNs: stepDur.Nanoseconds(),
		ProcTimeNs:     procTime.Nanoseconds(),
		FailThreshold:  failThreshold,
		BreakpointStep: -1,
	}

	// Tuned transport — same as TestPerfReport_5K so the saturation point
	// reflects platform behaviour, not connection-pool exhaustion.
	tr := &http.Transport{
		MaxIdleConns:        1024,
		MaxIdleConnsPerHost: 1024,
		MaxConnsPerHost:     1024,
		IdleConnTimeout:     90 * time.Second,
	}

	for i, perWorkerRPS := range steps {
		hist := hdrhistogram.New(1, 60_000_000_000, 3)
		var histMu sync.Mutex
		var reqs atomic.Int64
		var errs atomic.Int64

		cli := client.New(client.Config{BaseURL: srv.URL, Timeout: 2 * time.Second, Transport: tr})

		ctx, cancel := context.WithTimeout(context.Background(), stepDur)
		stepStart := time.Now()
		var wg sync.WaitGroup
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				interval := time.Duration(float64(time.Second) / perWorkerRPS)
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						start := time.Now()
						_, err := cli.PlaceOrder(ctx, "buy", 100.0, 1)
						reqs.Add(1)
						if err != nil {
							errs.Add(1)
							continue
						}
						lat := time.Since(start).Nanoseconds()
						if lat <= 0 {
							lat = 1
						}
						histMu.Lock()
						_ = hist.RecordValue(lat)
						histMu.Unlock()
					}
				}
			}()
		}
		wg.Wait()
		elapsed := time.Since(stepStart).Seconds()
		cancel()

		totalReqs := reqs.Load()
		totalErrs := errs.Load()
		errRate := 0.0
		if totalReqs > 0 {
			errRate = float64(totalErrs) / float64(totalReqs)
		}
		sr := stepReport{
			TargetAggregateRPS: perWorkerRPS * float64(workers),
			AchievedRPS:        float64(totalReqs) / elapsed,
			Requests:           totalReqs,
			Errors:             totalErrs,
			ErrorRate:          errRate,
			P50Ns:              hist.ValueAtQuantile(50),
			P99Ns:              hist.ValueAtQuantile(99),
		}
		r.Steps = append(r.Steps, sr)
		t.Logf("step %d/%d: target=%.0f aggregate RPS · achieved=%.0f · errors=%.2f%% · p99=%dns",
			i+1, len(steps), sr.TargetAggregateRPS, sr.AchievedRPS, errRate*100, sr.P99Ns)

		if errRate <= failThreshold {
			r.LastGoodRPS = sr.AchievedRPS
		}
	}

	// Breakpoint = first step that exceeds threshold AND is not followed by a
	// recovery (next step ≤ threshold). This filters out the cold-start /
	// Windows-ticker noise visible at very low RPS where the system clearly
	// isn't saturated. If the curve only blips once and then recovers, the
	// platform never hit a true breakpoint within the tested range.
	for i := range r.Steps {
		if r.Steps[i].ErrorRate <= r.FailThreshold {
			continue
		}
		// Look ahead one step. If next exists and recovered, treat this as noise.
		if i+1 < len(r.Steps) && r.Steps[i+1].ErrorRate <= r.FailThreshold {
			continue
		}
		r.BreakpointStep = i
		r.BreakpointRPS = r.Steps[i].AchievedRPS
		break
	}

	// Persist report to JSON for the docs.
	f, err := os.Create(reportPath)
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		t.Fatalf("encode report: %v", err)
	}
	t.Logf("saturation report written to %s", reportPath)
	t.Logf("last good aggregate RPS (≤%.0f%% errors): %.0f", failThreshold*100, r.LastGoodRPS)
	if r.BreakpointStep >= 0 {
		t.Logf("breakpoint at step %d: %.0f aggregate RPS (errors first exceeded threshold here)",
			r.BreakpointStep+1, r.BreakpointRPS)
	} else {
		t.Logf("no breakpoint reached within the ramp; raise the top step or lower SAT_PROC_TIME")
	}
}
