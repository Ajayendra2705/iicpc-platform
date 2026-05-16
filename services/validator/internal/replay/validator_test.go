package replay_test

import (
	"testing"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/replay"
)

func event(contestantID, orderID string, otype telemetryv1.OrderType, price float64, qty, filled, ts int64) *telemetryv1.OrderEvent {
	return &telemetryv1.OrderEvent{
		ContestantId:   contestantID,
		OrderId:        orderID,
		Type:           otype,
		Price:          price,
		Quantity:       qty,
		FilledQuantity: filled,
		SentTsNs:       ts,
	}
}

func TestPerfectCorrectness(t *testing.T) {
	v := replay.NewValidator()
	// sell first, then buy that fully fills the sell
	v.Process(event("c1", "s1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 99, 5, 0, 1))
	v.Process(event("c1", "b1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 100, 5, 5, 2))

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
	v.Process(event("c1", "s1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 99, 5, 0, 1))
	// contestant claims a fill of 10, but reference book can only fill 5
	v.Process(event("c1", "b1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 100, 10, 10, 2))

	r, _ := v.Report("c1")
	if r.Mismatches != 1 {
		t.Errorf("mismatches: got %d want 1", r.Mismatches)
	}
	if r.Correctness != 0.5 {
		t.Errorf("correctness: got %v want 0.5", r.Correctness)
	}
}

func TestPerContestantIsolation(t *testing.T) {
	v := replay.NewValidator()
	v.Process(event("c1", "s1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 99, 5, 0, 1))
	v.Process(event("c2", "s2", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 99, 5, 0, 1))
	v.Process(event("c1", "b1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 100, 5, 5, 2))
	v.Process(event("c2", "b2", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 100, 5, 0, 2)) // wrong claim

	r1, _ := v.Report("c1")
	r2, _ := v.Report("c2")
	if r1.Mismatches != 0 {
		t.Errorf("c1 mismatches: got %d", r1.Mismatches)
	}
	if r2.Mismatches != 1 {
		t.Errorf("c2 mismatches: got %d", r2.Mismatches)
	}
}

func TestCancelDoesNotAffectCounters(t *testing.T) {
	v := replay.NewValidator()
	v.Process(event("c1", "s1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 99, 5, 0, 1))
	v.Process(event("c1", "s1", telemetryv1.OrderType_ORDER_TYPE_CANCEL, 0, 0, 0, 2))

	r, _ := v.Report("c1")
	if r.TotalChecked != 1 {
		t.Errorf("total: got %d want 1 (cancel shouldn't count)", r.TotalChecked)
	}
}

func TestAllReports(t *testing.T) {
	v := replay.NewValidator()
	v.Process(event("c1", "b1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 100, 5, 0, 1))
	v.Process(event("c2", "b1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 100, 5, 0, 1))
	v.Process(event("c3", "b1", telemetryv1.OrderType_ORDER_TYPE_LIMIT, 100, 5, 0, 1))

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
