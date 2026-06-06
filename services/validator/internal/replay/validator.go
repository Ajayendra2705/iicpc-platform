package replay

import (
	"sync"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Report is the per-submission correctness summary. SubmissionID is empty for
// legacy events that carry no submission (those are keyed by contestant).
type Report struct {
	ContestantID string  `json:"contestant_id"`
	SubmissionID string  `json:"submission_id,omitempty"`
	TotalChecked int64   `json:"total_checked"`
	Mismatches   int64   `json:"mismatches"`
	Correctness  float64 `json:"correctness"` // 1.0 = perfect, 0.0 = always wrong
}

// Validator replays each submission's order stream through an in-process
// reference orderbook and compares the contestant-reported FilledQuantity
// against what the reference engine computed. Mismatches drop correctness.
// State is keyed by submission_id so a re-submission replays against a FRESH
// book and counters — an attempt never inherits the previous attempt's resting
// orders or mismatch tally. Legacy events with no submission fall back to
// keying by contestant_id, preserving the original behaviour.
type Validator struct {
	mu       sync.Mutex
	books    map[string]*Book
	counters map[string]*counters
}

type counters struct {
	contestantID string
	submissionID string
	total        int64
	mismatches   int64
}

func NewValidator() *Validator {
	return &Validator{
		books:    make(map[string]*Book),
		counters: make(map[string]*counters),
	}
}

// runKey returns the state key for an event and the identity tags it carries.
// Keying prefers submission_id (per-attempt isolation) and falls back to
// contestant_id for legacy callers that don't set a submission.
func runKey(e *telemetryv1.OrderEvent) (key, contestant, submission string) {
	contestant = e.GetContestantId()
	submission = e.GetSubmissionId()
	if submission != "" {
		return submission, contestant, submission
	}
	return contestant, contestant, submission
}

// Process applies a single OrderEvent: places (or cancels) on the submission's
// reference book and compares the expected fill against the contestant report.
func (v *Validator) Process(e *telemetryv1.OrderEvent) {
	if e == nil || e.GetContestantId() == "" {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	key, contestant, submission := runKey(e)
	book, ok := v.books[key]
	if !ok {
		book = New()
		v.books[key] = book
	}
	c, ok := v.counters[key]
	if !ok {
		c = &counters{contestantID: contestant, submissionID: submission}
		v.counters[key] = c
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

// Report returns the correctness summary for one run key (a submission_id, or
// a contestant_id for legacy runs).
func (v *Validator) Report(key string) (Report, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	c, ok := v.counters[key]
	if !ok {
		return Report{}, false
	}
	return makeReport(c), true
}

// All returns reports for every run (submission, or contestant for legacy
// events) seen so far.
func (v *Validator) All() []Report {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]Report, 0, len(v.counters))
	for _, c := range v.counters {
		out = append(out, makeReport(c))
	}
	return out
}

func makeReport(c *counters) Report {
	correctness := 1.0
	if c.total > 0 {
		correctness = 1.0 - float64(c.mismatches)/float64(c.total)
	}
	return Report{
		ContestantID: c.contestantID,
		SubmissionID: c.submissionID,
		TotalChecked: c.total,
		Mismatches:   c.mismatches,
		Correctness:  correctness,
	}
}
