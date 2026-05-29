package replay

import (
	"sync"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Report is the per-contestant correctness summary.
type Report struct {
	ContestantID string  `json:"contestant_id"`
	TotalChecked int64   `json:"total_checked"`
	Mismatches   int64   `json:"mismatches"`
	Correctness  float64 `json:"correctness"` // 1.0 = perfect, 0.0 = always wrong
}

// Validator replays each contestant's order stream through an in-process
// reference orderbook and compares the contestant-reported FilledQuantity
// against what the reference engine computed. Mismatches drop correctness.
type Validator struct {
	mu       sync.Mutex
	books    map[string]*Book
	counters map[string]*counters
}

type counters struct {
	total      int64
	mismatches int64
}

func NewValidator() *Validator {
	return &Validator{
		books:    make(map[string]*Book),
		counters: make(map[string]*counters),
	}
}

// Process applies a single OrderEvent: places (or cancels) on the contestant's
// reference book and compares the expected fill against the contestant report.
func (v *Validator) Process(e *telemetryv1.OrderEvent) {
	if e == nil || e.GetContestantId() == "" {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	cid := e.GetContestantId()
	book, ok := v.books[cid]
	if !ok {
		book = New()
		v.books[cid] = book
	}
	c, ok := v.counters[cid]
	if !ok {
		c = &counters{}
		v.counters[cid] = c
	}

	switch e.GetType() {
	case telemetryv1.OrderType_ORDER_TYPE_LIMIT, telemetryv1.OrderType_ORDER_TYPE_MARKET:
		side := Buy
		if e.GetSide() == telemetryv1.OrderSide_ORDER_SIDE_SELL {
			side = Sell
		}

		// Replay against the reference book to compute the canonical fill.
		// Market orders are IOC (match-and-discard); limit orders rest.
		var expected int64
		if e.GetType() == telemetryv1.OrderType_ORDER_TYPE_MARKET {
			expected = book.PlaceMarket(side, e.GetQuantity())
		} else {
			expected = book.Place(Order{
				ID:    e.GetOrderId(),
				Side:  side,
				Price: e.GetPrice(),
				Qty:   e.GetQuantity(),
				TsNs:  e.GetSentTsNs(),
			})
		}

		// Only score fill accuracy when the bot reported an authoritative fill.
		// An UNSPECIFIED result means the transport (e.g. FIX) didn't capture a
		// fill, so we replayed only to keep book state consistent.
		if e.GetResult() != telemetryv1.OrderResult_ORDER_RESULT_UNSPECIFIED {
			c.total++
			if expected != e.GetFilledQuantity() {
				c.mismatches++
			}
		}
	case telemetryv1.OrderType_ORDER_TYPE_CANCEL:
		book.Cancel(e.GetOrderId())
	}
}

// Report returns the correctness summary for one contestant.
func (v *Validator) Report(contestantID string) (Report, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	c, ok := v.counters[contestantID]
	if !ok {
		return Report{}, false
	}
	return makeReport(contestantID, c), true
}

// All returns reports for every contestant seen so far.
func (v *Validator) All() []Report {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]Report, 0, len(v.counters))
	for cid, c := range v.counters {
		out = append(out, makeReport(cid, c))
	}
	return out
}

func makeReport(cid string, c *counters) Report {
	correctness := 1.0
	if c.total > 0 {
		correctness = 1.0 - float64(c.mismatches)/float64(c.total)
	}
	return Report{
		ContestantID: cid,
		TotalChecked: c.total,
		Mismatches:   c.mismatches,
		Correctness:  correctness,
	}
}
