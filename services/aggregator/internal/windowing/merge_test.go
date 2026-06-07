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

// TestAllMergedScoresWholeRunNotLastWindow is the regression test for the
// scoring fix: the leaderboard must rank on the whole-run merged histogram,
// not the most recent 1-second window. A contestant that was fast for one
// window then slow for the next must surface its true (slow) tail latency.
func TestAllMergedReturnsWholeRunPerContestant(t *testing.T) {
	a := windowing.New(time.Second)

	// c1: window of fast (1ms) then window of slow (100ms).
	for range 500 {
		a.Record(&telemetryv1.OrderEvent{ContestantId: "c1", LatencyNs: 1_000_000})
	}
	a.Flush(time.Now())
	for range 500 {
		a.Record(&telemetryv1.OrderEvent{ContestantId: "c1", LatencyNs: 100_000_000})
	}
	a.Flush(time.Now())

	// c2: single steady window.
	for range 300 {
		a.Record(&telemetryv1.OrderEvent{ContestantId: "c2", LatencyNs: 5_000_000})
	}
	a.Flush(time.Now())

	rows := a.AllMerged(0) // 0 → entire retained history
	if len(rows) != 2 {
		t.Fatalf("AllMerged: got %d rows, want 2", len(rows))
	}

	byID := map[string]windowing.MergedSnapshot{}
	for _, m := range rows {
		byID[m.ContestantID] = m
	}

	c1 := byID["c1"]
	if c1.TotalCount != 1000 {
		t.Errorf("c1 TotalCount: got %d want 1000 (both windows merged)", c1.TotalCount)
	}
	// Whole-run P99 must reflect the slow window (~100ms), not just the last
	// window's own P99 — but more importantly it merges both windows' data.
	if c1.P99Ns < 90_000_000 {
		t.Errorf("c1 merged P99: got %d ns, expected ~100ms from the merged run", c1.P99Ns)
	}
	if c1.WindowCount != 2 {
		t.Errorf("c1 WindowCount: got %d want 2", c1.WindowCount)
	}

	c2 := byID["c2"]
	if c2.TotalCount != 300 || c2.WindowCount != 1 {
		t.Errorf("c2 merged: count=%d windows=%d, want 300/1", c2.TotalCount, c2.WindowCount)
	}
}

// TestSubmissionsAreScoredInIsolation proves the re-submission fix: two
// submissions from the SAME contestant must not blend into one histogram. A
// fast first attempt and a slow second attempt yield two separate merged rows,
// each carrying its own submission_id and its own (un-blended) percentiles.
func TestSubmissionsAreScoredInIsolation(t *testing.T) {
	a := windowing.New(time.Second)

	// Attempt sub-1: fast (~1ms).
	for range 500 {
		a.Record(&telemetryv1.OrderEvent{ContestantId: "c1", SubmissionId: "sub-1", LatencyNs: 1_000_000})
	}
	a.Flush(time.Now())
	// Attempt sub-2 by the SAME contestant: slow (~100ms).
	for range 500 {
		a.Record(&telemetryv1.OrderEvent{ContestantId: "c1", SubmissionId: "sub-2", LatencyNs: 100_000_000})
	}
	a.Flush(time.Now())

	rows := a.AllMerged(0)
	if len(rows) != 2 {
		t.Fatalf("AllMerged: got %d rows, want 2 (one per submission)", len(rows))
	}
	bySub := map[string]windowing.MergedSnapshot{}
	for _, m := range rows {
		if m.ContestantID != "c1" {
			t.Errorf("submission %s: contestant=%q want c1", m.SubmissionID, m.ContestantID)
		}
		bySub[m.SubmissionID] = m
	}

	s1, ok1 := bySub["sub-1"]
	s2, ok2 := bySub["sub-2"]
	if !ok1 || !ok2 {
		t.Fatalf("missing a submission row: got keys %v", bySub)
	}
	// Each attempt keeps its own count — no blending into 1000.
	if s1.TotalCount != 500 || s2.TotalCount != 500 {
		t.Errorf("counts blended: sub-1=%d sub-2=%d, want 500/500", s1.TotalCount, s2.TotalCount)
	}
	// The fast attempt's P99 stays fast; the slow attempt's stays slow. If they
	// had blended, sub-1's P99 would be dragged toward 100ms.
	if s1.P99Ns > 5_000_000 {
		t.Errorf("sub-1 P99=%d ns; expected ~1ms (blended with slow attempt?)", s1.P99Ns)
	}
	if s2.P99Ns < 90_000_000 {
		t.Errorf("sub-2 P99=%d ns; expected ~100ms", s2.P99Ns)
	}
}

// TestLegacyEventsKeyByContestant confirms back-compat: events with no
// submission_id still bucket by contestant_id exactly as before.
func TestLegacyEventsKeyByContestant(t *testing.T) {
	a := windowing.New(time.Second)
	for range 200 {
		a.Record(&telemetryv1.OrderEvent{ContestantId: "legacy", LatencyNs: 2_000_000})
	}
	a.Flush(time.Now())

	rows := a.AllMerged(0)
	if len(rows) != 1 {
		t.Fatalf("AllMerged: got %d rows, want 1", len(rows))
	}
	if rows[0].ContestantID != "legacy" || rows[0].SubmissionID != "" {
		t.Errorf("legacy keying: contestant=%q submission=%q, want legacy/empty",
			rows[0].ContestantID, rows[0].SubmissionID)
	}
	if rows[0].TotalCount != 200 {
		t.Errorf("legacy count: got %d want 200", rows[0].TotalCount)
	}
}

func TestAllMergedEmptyBeforeAnyFlush(t *testing.T) {
	a := windowing.New(time.Second)
	a.Record(&telemetryv1.OrderEvent{ContestantId: "c1", LatencyNs: 1_000_000})
	// No Flush yet → nothing in history → no merged rows.
	if rows := a.AllMerged(0); len(rows) != 0 {
		t.Errorf("AllMerged before flush: got %d rows, want 0", len(rows))
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

// TestAllMergedScoresEntireRunBeyondHistoryCap is the regression test for the
// whole-run scoring fix. The bounded history ring only retains the last
// historyCap windows, so a run longer than that would silently drop its early
// performance from the score. AllMerged(0) must instead read the never-evicted
// cumulative accumulator and count EVERY window, while the ring view stays
// capped.
func TestAllMergedScoresEntireRunBeyondHistoryCap(t *testing.T) {
	a := windowing.New(time.Second)

	total := windowing.DefaultHistoryWindows + 40 // run longer than the ring
	for range total {
		a.Record(&telemetryv1.OrderEvent{ContestantId: "c1", LatencyNs: 1_000_000})
		a.Flush(time.Now())
	}

	// Scoring path: whole run, not capped at historyCap.
	rows := a.AllMerged(0)
	if len(rows) != 1 {
		t.Fatalf("AllMerged: got %d rows, want 1", len(rows))
	}
	if rows[0].WindowCount != total {
		t.Errorf("whole-run WindowCount: got %d want %d (early windows dropped?)", rows[0].WindowCount, total)
	}
	if rows[0].TotalCount != int64(total) {
		t.Errorf("whole-run TotalCount: got %d want %d", rows[0].TotalCount, total)
	}

	// Recent-window view stays bounded by the ring.
	ringView, _ := a.MergedRecent("c1", 10000)
	if ringView.WindowCount > windowing.DefaultHistoryWindows {
		t.Errorf("ring view WindowCount: got %d, want <= cap %d", ringView.WindowCount, windowing.DefaultHistoryWindows)
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
