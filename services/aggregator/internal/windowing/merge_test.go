package windowing_test

import (
	"testing"
	"time"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

// TestMergeIsStrictlyMoreAccurateThanAveraging documents *why* the merge
// matters. Two windows, one with tiny latencies and one with huge latencies.
// Averaging their P99 understates the merged truth.
func TestMergeIsStrictlyMoreAccurateThanAveraging(t *testing.T) {
	a := windowing.New(time.Second)

	// Window 1: 1000 fast events (~1ms)
	for range 1000 {
		a.Record(&telemetryv1.OrderEvent{
			ContestantId: "c1", LatencyNs: 1_000_000, // 1ms
			Result: telemetryv1.OrderResult_ORDER_RESULT_ACK_ONLY,
		})
	}
	a.Flush(time.Now())

	// Window 2: 1000 slow events (~100ms)
	for range 1000 {
		a.Record(&telemetryv1.OrderEvent{
			ContestantId: "c1", LatencyNs: 100_000_000, // 100ms
			Result: telemetryv1.OrderResult_ORDER_RESULT_ACK_ONLY,
		})
	}
	a.Flush(time.Now())

	s1, _ := a.Latest("c1") // latest only sees window 2
	if s1.P99Ns < 90_000_000 || s1.P99Ns > 110_000_000 {
		t.Errorf("latest P99 in window 2 expected ~100ms, got %d ns", s1.P99Ns)
	}

	merged, ok := a.MergedRecent("c1", 2)
	if !ok {
		t.Fatal("MergedRecent returned no result")
	}
	if merged.WindowCount != 2 {
		t.Errorf("WindowCount: got %d want 2", merged.WindowCount)
	}
	if merged.TotalCount != 2000 {
		t.Errorf("TotalCount: got %d want 2000", merged.TotalCount)
	}
	// Merged P99 of {1000×1ms, 1000×100ms} is the 1980th sorted value,
	// which is in the 100ms bucket. NOT the average of P99s (~50ms).
	if merged.P99Ns < 90_000_000 {
		t.Errorf("merged P99: got %d ns, expected ~100ms (averaging would have given ~50ms)", merged.P99Ns)
	}
	// Merged P50 across 1000 fast + 1000 slow is the 1000th sorted value,
	// which is on the boundary — should be near 1ms (last of the fast bucket).
	if merged.P50Ns > 5_000_000 {
		t.Errorf("merged P50: got %d ns, expected ~1ms", merged.P50Ns)
	}
}

func TestMergedRecentRespectsWindowsArg(t *testing.T) {
	a := windowing.New(time.Second)
	for i := range 5 {
		for range 100 {
			a.Record(&telemetryv1.OrderEvent{
				ContestantId: "c1",
				LatencyNs:    int64(1_000_000 * (i + 1)), // 1ms, 2ms, ..., 5ms
				Result:       telemetryv1.OrderResult_ORDER_RESULT_ACK_ONLY,
			})
		}
		a.Flush(time.Now())
	}

	// Last 2 windows only — events were 4ms and 5ms
	merged2, _ := a.MergedRecent("c1", 2)
	if merged2.WindowCount != 2 || merged2.TotalCount != 200 {
		t.Errorf("merged(2): got windows=%d count=%d", merged2.WindowCount, merged2.TotalCount)
	}
	if merged2.P50Ns < 3_500_000 {
		t.Errorf("merged(2) P50: got %d, expected ≥ 4ms", merged2.P50Ns)
	}

	// All 5 windows
	merged5, _ := a.MergedRecent("c1", 5)
	if merged5.WindowCount != 5 || merged5.TotalCount != 500 {
		t.Errorf("merged(5): got windows=%d count=%d", merged5.WindowCount, merged5.TotalCount)
	}
}

func TestMergedRecentNotFound(t *testing.T) {
	a := windowing.New(time.Second)
	if _, ok := a.MergedRecent("nobody", 60); ok {
		t.Error("MergedRecent on unknown contestant should return false")
	}
}

func TestMergedRecentClampsWindowsArg(t *testing.T) {
	a := windowing.New(time.Second)
	a.Record(&telemetryv1.OrderEvent{ContestantId: "c1", LatencyNs: 1_000_000})
	a.Flush(time.Now())
	// Request way more windows than exist — should clamp, not error
	merged, ok := a.MergedRecent("c1", 9999)
	if !ok || merged.WindowCount != 1 {
		t.Errorf("clamp: ok=%v windows=%d", ok, merged.WindowCount)
	}
}

func TestHistoryRingDoesNotGrowUnbounded(t *testing.T) {
	a := windowing.New(time.Second)
	// Push way more flushes than DefaultHistoryWindows
	for range windowing.DefaultHistoryWindows + 50 {
		a.Record(&telemetryv1.OrderEvent{ContestantId: "c1", LatencyNs: 1_000_000})
		a.Flush(time.Now())
	}
	merged, _ := a.MergedRecent("c1", 10000)
	if merged.WindowCount > windowing.DefaultHistoryWindows {
		t.Errorf("ring grew unbounded: %d > %d cap", merged.WindowCount, windowing.DefaultHistoryWindows)
	}
}
