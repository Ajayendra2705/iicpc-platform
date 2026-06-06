package replay_test

import (
	"testing"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/replay"
)

// event builds an OrderEvent with an authoritative fill result so the validator
// scores it. Side is now carried explicitly (no more order-id-prefix heuristic).
func event(contestantID, orderID string, otype telemetryv1.OrderType, side telemetryv1.OrderSide, price float64, qty, filled, ts int64) *telemetryv1.OrderEvent {
	result := telemetryv1.OrderResult_ORDER_RESULT_ACK_ONLY
	switch {
	case qty > 0 && filled >= qty:
		result = telemetryv1.OrderResult_ORDER_RESULT_FILLED
	case filled > 0:
		result = telemetryv1.OrderResult_ORDER_RESULT_PARTIAL
	}
	return &telemetryv1.OrderEvent{
		ContestantId:   contestantID,
		OrderId:        orderID,
		Type:           otype,
		Side:           side,
		Price:          price,
		Quantity:       qty,
		FilledQuantity: filled,
		Result:         result,
		SentTsNs:       ts,
	}
}

const (
	buy  = telemetryv1.OrderSide_ORDER_SIDE_BUY
	sell = telemetryv1.OrderSide_ORDER_SIDE_SELL
	lim  = telemetryv1.OrderType_ORDER_TYPE_LIMIT
	mkt  = telemetryv1.OrderType_ORDER_TYPE_MARKET
	cxl  = telemetryv1.OrderType_ORDER_TYPE_CANCEL
)

func TestPerfectCorrectness(t *testing.T) {
	v := replay.NewValidator()
	// sell first, then buy that fully fills the sell
	v.Process(event("c1", "s1", lim, sell, 99, 5, 0, 1))
	v.Process(event("c1", "b1", lim, buy, 100, 5, 5, 2))

	r, ok := v.Report("c1")
	if !ok {
		t.Fatal("no report")
	}
	if r.TotalChecked != 2 || r.Mismatches != 0 {
		t.Errorf("report: %+v", r)
	}
	if r.Correctness != 1.0 {
		t.Errorf("correctness: got %v want 1.0", r.Correctness)
	}
}

func TestMismatchDetected(t *testing.T) {
	v := replay.NewValidator()
	v.Process(event("c1", "s1", lim, sell, 99, 5, 0, 1))
	// contestant claims a fill of 10, but reference book can only fill 5
	v.Process(event("c1", "b1", lim, buy, 100, 10, 10, 2))

	r, _ := v.Report("c1")
	if r.Mismatches != 1 {
		t.Errorf("mismatches: got %d want 1", r.Mismatches)
	}
	if r.Correctness != 0.5 {
		t.Errorf("correctness: got %v want 0.5", r.Correctness)
	}
}

func TestMarketOrderFillScored(t *testing.T) {
	v := replay.NewValidator()
	// rest a sell at 99 qty 5, then a market buy for 8 → IOC fills 5, no rest.
	v.Process(event("c1", "s1", lim, sell, 99, 5, 0, 1))
	v.Process(event("c1", "m1", mkt, buy, 0, 8, 5, 2))

	r, _ := v.Report("c1")
	if r.TotalChecked != 2 || r.Mismatches != 0 {
		t.Errorf("market fill should match (expected 5): %+v", r)
	}
}

func TestPerContestantIsolation(t *testing.T) {
	v := replay.NewValidator()
	v.Process(event("c1", "s1", lim, sell, 99, 5, 0, 1))
	v.Process(event("c2", "s2", lim, sell, 99, 5, 0, 1))
	v.Process(event("c1", "b1", lim, buy, 100, 5, 5, 2))
	v.Process(event("c2", "b2", lim, buy, 100, 5, 0, 2)) // wrong claim

	r1, _ := v.Report("c1")
	r2, _ := v.Report("c2")
	if r1.Mismatches != 0 {
		t.Errorf("c1 mismatches: got %d", r1.Mismatches)
	}
	if r2.Mismatches != 1 {
		t.Errorf("c2 mismatches: got %d", r2.Mismatches)
	}
}

// withSub stamps a submission_id onto an event built by the event helper.
func withSub(e *telemetryv1.OrderEvent, sid string) *telemetryv1.OrderEvent {
	e.SubmissionId = sid
	return e
}

// TestPerSubmissionIsolation proves a contestant's two attempts replay against
// SEPARATE reference books and keep SEPARATE correctness tallies — a buggy
// second attempt must not taint the clean first attempt, and the clean attempt's
// resting orders must not leak into the second attempt's book.
func TestPerSubmissionIsolation(t *testing.T) {
	v := replay.NewValidator()
	// Attempt s1 (clean): rest a sell, then a buy that correctly fills 5.
	v.Process(withSub(event("c1", "s1-sell", lim, sell, 99, 5, 0, 1), "s1"))
	v.Process(withSub(event("c1", "s1-buy", lim, buy, 100, 5, 5, 2), "s1"))
	// Attempt s2 (buggy): a buy claims a fill of 5, but s2's book is EMPTY
	// (s1's resting sell must not leak in), so the correct fill is 0 → mismatch.
	v.Process(withSub(event("c1", "s2-buy", lim, buy, 100, 5, 5, 3), "s2"))

	r1, ok1 := v.Report("s1")
	r2, ok2 := v.Report("s2")
	if !ok1 || !ok2 {
		t.Fatalf("missing report: s1=%v s2=%v", ok1, ok2)
	}
	if r1.ContestantID != "c1" || r1.SubmissionID != "s1" {
		t.Errorf("s1 identity: %+v", r1)
	}
	if r1.Mismatches != 0 || r1.Correctness != 1.0 {
		t.Errorf("s1 should be perfect (own book): %+v", r1)
	}
	if r2.Mismatches != 1 || r2.Correctness != 0.0 {
		t.Errorf("s2 should catch the bogus fill (fresh empty book): %+v", r2)
	}
}

func TestUnspecifiedResultNotScored(t *testing.T) {
	v := replay.NewValidator()
	// A FIX-style event with no authoritative fill: replayed for book state but
	// not counted toward correctness.
	e := event("c1", "b1", lim, buy, 100, 5, 0, 1)
	e.Result = telemetryv1.OrderResult_ORDER_RESULT_UNSPECIFIED
	v.Process(e)

	r, _ := v.Report("c1")
	if r.TotalChecked != 0 {
		t.Fatalf("unspecified-result event must not be scored: TotalChecked=%d want 0", r.TotalChecked)
	}
}

func TestCancelDoesNotAffectCounters(t *testing.T) {
	v := replay.NewValidator()
	v.Process(event("c1", "s1", lim, sell, 99, 5, 0, 1))
	v.Process(event("c1", "s1", cxl, sell, 0, 0, 0, 2))

	r, _ := v.Report("c1")
	if r.TotalChecked != 1 {
		t.Errorf("total: got %d want 1 (cancel shouldn't count)", r.TotalChecked)
	}
}

func TestAllReports(t *testing.T) {
	v := replay.NewValidator()
	v.Process(event("c1", "b1", lim, buy, 100, 5, 0, 1))
	v.Process(event("c2", "b1", lim, buy, 100, 5, 0, 1))
	v.Process(event("c3", "b1", lim, buy, 100, 5, 0, 1))

	all := v.All()
	if len(all) != 3 {
		t.Errorf("All: got %d want 3", len(all))
	}
}

func TestReportNotFound(t *testing.T) {
	v := replay.NewValidator()
	if _, ok := v.Report("nope"); ok {
		t.Error("expected not found")
	}
}

func TestProcessIgnoresNilOrMissingID(t *testing.T) {
	v := replay.NewValidator()
	v.Process(nil)
	v.Process(&telemetryv1.OrderEvent{OrderId: "x"})
	if got := len(v.All()); got != 0 {
		t.Errorf("All: got %d want 0", got)
	}
}
