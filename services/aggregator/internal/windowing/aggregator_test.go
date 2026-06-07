package windowing_test

import (
	"testing"
	"time"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

func evt(contestantID string, latNs int64, result telemetryv1.OrderResult) *telemetryv1.OrderEvent {
	return &telemetryv1.OrderEvent{
		ContestantId: contestantID,
		LatencyNs:    latNs,
		Result:       result,
	}
}

func evtSub(contestantID, submissionID string, latNs int64, result telemetryv1.OrderResult) *telemetryv1.OrderEvent {
	return &telemetryv1.OrderEvent{
		ContestantId: contestantID,
		SubmissionId: submissionID,
		LatencyNs:    latNs,
		Result:       result,
	}
}

// When attempts carry a submission_id the run maps are keyed by submission, so
// the per-contestant detail endpoints (Latest / MergedRecent by contestant_id)
// must still resolve to the contestant's most recent submission.
func TestLatestAndMergedResolveContestantWithSubmissionID(t *testing.T) {
	a := windowing.New(time.Second)

	// First attempt: slow.
	for range 20 {
		a.Record(evtSub("c1", "sub-old", 9_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	}
	a.Flush(time.Now().Add(-time.Minute)) // older window

	// Second (latest) attempt: faster, fewer orders.
	for range 5 {
		a.Record(evtSub("c1", "sub-new", 1_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	}
	a.Flush(time.Now())

	// Direct submission_id lookups still work.
	if _, ok := a.Latest("sub-new"); !ok {
		t.Fatal("Latest by submission_id: not found")
	}

	// Lookup by contestant_id must resolve to the most recent submission.
	got, ok := a.Latest("c1")
	if !ok {
		t.Fatal("Latest by contestant_id: not found")
	}
	if got.SubmissionID != "sub-new" {
		t.Errorf("Latest resolved to %q, want most recent submission sub-new", got.SubmissionID)
	}
	if got.Count != 5 {
		t.Errorf("Latest count: got %d want 5 (the latest attempt)", got.Count)
	}

	merged, ok := a.MergedRecent("c1", 0)
	if !ok {
		t.Fatal("MergedRecent by contestant_id: not found")
	}
	if merged.SubmissionID != "sub-new" {
		t.Errorf("MergedRecent resolved to %q, want sub-new", merged.SubmissionID)
	}
	if merged.TotalCount != 5 {
		t.Errorf("MergedRecent total count: got %d want 5", merged.TotalCount)
	}
}

func TestPercentileOrdering(t *testing.T) {
	a := windowing.New(time.Second)
	for i := int64(1); i <= 1000; i++ {
		a.Record(evt("c1", i*1_000_000, telemetryv1.OrderResult_ORDER_RESULT_ACK_ONLY)) // 1ms..1s
	}
	snaps := a.Flush(time.Now())
	if len(snaps) != 1 {
		t.Fatalf("snapshots: got %d want 1", len(snaps))
	}
	s := snaps[0]
	if s.Count != 1000 {
		t.Errorf("count: got %d want 1000", s.Count)
	}
	if !(s.P50Ns < s.P90Ns && s.P90Ns < s.P99Ns && s.P99Ns < s.P999Ns) {
		t.Errorf("percentiles not strictly increasing: p50=%d p90=%d p99=%d p999=%d",
			s.P50Ns, s.P90Ns, s.P99Ns, s.P999Ns)
	}
	// p50 should be ~500ms (500_000_000 ns), allow 1% drift for HDR precision
	want := int64(500_000_000)
	if abs64(s.P50Ns-want)*100/want > 1 {
		t.Errorf("p50: got %d ns, want ~%d ns (±1%%)", s.P50Ns, want)
	}
}

func TestPerContestantIsolation(t *testing.T) {
	a := windowing.New(time.Second)
	for range 100 {
		a.Record(evt("c1", 1_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	}
	for range 50 {
		a.Record(evt("c2", 2_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	}
	snaps := a.Flush(time.Now())
	if len(snaps) != 2 {
		t.Fatalf("snapshots: got %d want 2", len(snaps))
	}
	for _, s := range snaps {
		switch s.ContestantID {
		case "c1":
			if s.Count != 100 {
				t.Errorf("c1 count: got %d want 100", s.Count)
			}
		case "c2":
			if s.Count != 50 {
				t.Errorf("c2 count: got %d want 50", s.Count)
			}
		default:
			t.Errorf("unexpected contestant: %s", s.ContestantID)
		}
	}
}

func TestErrorCounting(t *testing.T) {
	a := windowing.New(time.Second)
	a.Record(evt("c1", 1_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	a.Record(evt("c1", 2_000_000, telemetryv1.OrderResult_ORDER_RESULT_REJECTED))
	a.Record(evt("c1", 3_000_000, telemetryv1.OrderResult_ORDER_RESULT_REJECTED))
	a.Record(evt("c1", 4_000_000, telemetryv1.OrderResult_ORDER_RESULT_TIMEOUT))

	snaps := a.Flush(time.Now())
	if len(snaps) != 1 {
		t.Fatalf("snapshots: got %d want 1", len(snaps))
	}
	s := snaps[0]
	if s.Count != 4 {
		t.Errorf("count: got %d want 4", s.Count)
	}
	if s.Rejected != 2 {
		t.Errorf("rejected: got %d want 2", s.Rejected)
	}
	if s.Timeouts != 1 {
		t.Errorf("timeouts: got %d want 1", s.Timeouts)
	}
}

func TestFlushResetsWindow(t *testing.T) {
	a := windowing.New(time.Second)
	a.Record(evt("c1", 1_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	a.Flush(time.Now())
	// second flush with no events should produce empty result but preserve latest
	snaps := a.Flush(time.Now())
	if len(snaps) != 0 {
		t.Errorf("second flush: expected 0 snapshots, got %d", len(snaps))
	}
	if _, ok := a.Latest("c1"); !ok {
		t.Error("latest snapshot lost after second flush")
	}
}

func TestLatestReturnsMostRecent(t *testing.T) {
	a := windowing.New(time.Second)
	a.Record(evt("c1", 1_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	a.Flush(time.Now())
	for range 5 {
		a.Record(evt("c1", 5_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	}
	a.Flush(time.Now())

	got, ok := a.Latest("c1")
	if !ok {
		t.Fatal("Latest: not found")
	}
	if got.Count != 5 {
		t.Errorf("latest count: got %d want 5", got.Count)
	}
}

func TestLatestNotFound(t *testing.T) {
	a := windowing.New(time.Second)
	if _, ok := a.Latest("nope"); ok {
		t.Error("expected not found for unknown contestant")
	}
}

func TestAllReturnsAllContestants(t *testing.T) {
	a := windowing.New(time.Second)
	a.Record(evt("c1", 1_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	a.Record(evt("c2", 2_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	a.Record(evt("c3", 3_000_000, telemetryv1.OrderResult_ORDER_RESULT_FILLED))
	a.Flush(time.Now())

	all := a.All()
	if len(all) != 3 {
		t.Errorf("All: got %d want 3", len(all))
	}
}

func TestRecordIgnoresNilOrMissingID(t *testing.T) {
	a := windowing.New(time.Second)
	a.Record(nil)
	a.Record(&telemetryv1.OrderEvent{LatencyNs: 1000}) // no contestant_id
	snaps := a.Flush(time.Now())
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots for invalid input, got %d", len(snaps))
	}
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
