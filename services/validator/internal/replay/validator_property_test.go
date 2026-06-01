package replay

import (
	"fmt"
	"math/rand"
	"testing"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Property-based tests for the Validator. Rather than a handful of hand-rolled
// fixtures (which can pass by luck), these generate thousands of randomised
// order streams and assert invariants that must hold for *every* stream:
//
//   P1 — no false positives: a contestant that reports exactly the reference
//        engine's fills scores correctness == 1.0, always.
//   P2 — no false negatives: flip any subset of reported fills and the
//        validator catches exactly that many mismatches.
//   P3 — determinism: the same stream scored twice yields identical reports
//        (no map-order / RNG leakage in the scoring path).
//
// The fill oracle here is the same reference Book the validator uses, so these
// pin the validator's *wiring* (side/type/market-vs-limit handling, result
// gating, counter math) and its determinism — the matcher's own correctness is
// covered by book_test.go. Differential testing against a second independent
// engine is tracked separately in IDEAS.md.

const propIterations = 2000

// streamEvent is a generated order plus the fill the reference engine computes
// for it (oracleFill), used to build a "correct" contestant report.
type streamEvent struct {
	ev         *telemetryv1.OrderEvent
	oracleFill int64
	scored     bool // LIMIT/MARKET with a determinate result → contributes to total
}

// genStream builds a random but self-consistent order stream for one contestant
// and computes each order's canonical fill via an independent oracle Book.
func genStream(rng *rand.Rand, cid string, n int) []streamEvent {
	oracle := New()
	mid := 100.0
	out := make([]streamEvent, 0, n)
	var liveIDs []string

	for i := range n {
		id := fmt.Sprintf("%s-o%d", cid, i)
		roll := rng.Intn(10)

		switch {
		case roll == 0 && len(liveIDs) > 0:
			// Cancel a previously-placed order.
			victim := liveIDs[rng.Intn(len(liveIDs))]
			oracle.Cancel(victim)
			out = append(out, streamEvent{
				ev: &telemetryv1.OrderEvent{
					ContestantId: cid,
					OrderId:      victim,
					Type:         telemetryv1.OrderType_ORDER_TYPE_CANCEL,
				},
				scored: false,
			})
		case roll <= 2:
			// Market order (IOC) — matches and discards remainder.
			side := pickSide(rng)
			qty := int64(rng.Intn(10) + 1)
			fill := oracle.PlaceMarket(toReplaySide(side), qty)
			out = append(out, streamEvent{
				ev: &telemetryv1.OrderEvent{
					ContestantId:   cid,
					OrderId:        id,
					Type:           telemetryv1.OrderType_ORDER_TYPE_MARKET,
					Side:           side,
					Quantity:       qty,
					FilledQuantity: fill,
					Result:         telemetryv1.OrderResult_ORDER_RESULT_FILLED,
					SentTsNs:       int64(i + 1),
				},
				oracleFill: fill,
				scored:     true,
			})
		default:
			// Limit order — crosses or rests near the mid price.
			side := pickSide(rng)
			price := mid + float64(rng.Intn(7)-3) // mid-3 .. mid+3
			qty := int64(rng.Intn(10) + 1)
			fill := oracle.Place(Order{
				ID:    id,
				Side:  toReplaySide(side),
				Price: price,
				Qty:   qty,
				TsNs:  int64(i + 1),
			})
			out = append(out, streamEvent{
				ev: &telemetryv1.OrderEvent{
					ContestantId:   cid,
					OrderId:        id,
					Type:           telemetryv1.OrderType_ORDER_TYPE_LIMIT,
					Side:           side,
					Price:          price,
					Quantity:       qty,
					FilledQuantity: fill,
					Result:         telemetryv1.OrderResult_ORDER_RESULT_FILLED,
					SentTsNs:       int64(i + 1),
				},
				oracleFill: fill,
				scored:     true,
			})
			liveIDs = append(liveIDs, id)
		}
	}
	return out
}

func pickSide(rng *rand.Rand) telemetryv1.OrderSide {
	if rng.Intn(2) == 0 {
		return telemetryv1.OrderSide_ORDER_SIDE_BUY
	}
	return telemetryv1.OrderSide_ORDER_SIDE_SELL
}

func toReplaySide(s telemetryv1.OrderSide) Side {
	if s == telemetryv1.OrderSide_ORDER_SIDE_SELL {
		return Sell
	}
	return Buy
}

func countScored(stream []streamEvent) int64 {
	var n int64
	for _, s := range stream {
		if s.scored {
			n++
		}
	}
	return n
}

// TestProperty_CorrectContestantAlwaysScoresPerfect asserts P1: a contestant
// reporting exactly the reference fills is never penalised, for any stream.
func TestProperty_CorrectContestantAlwaysScoresPerfect(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for iter := range propIterations {
		cid := fmt.Sprintf("c%d", iter)
		stream := genStream(rng, cid, rng.Intn(40)+1)

		v := NewValidator()
		for _, s := range stream {
			v.Process(s.ev)
		}
		rep, ok := v.Report(cid)
		if !ok && countScored(stream) > 0 {
			t.Fatalf("iter %d: no report for a stream with scored events", iter)
		}
		if countScored(stream) == 0 {
			continue
		}
		if rep.Mismatches != 0 || rep.Correctness != 1.0 {
			t.Fatalf("iter %d: correct contestant scored < perfect: %+v", iter, rep)
		}
		if rep.TotalChecked != countScored(stream) {
			t.Fatalf("iter %d: total_checked=%d, want %d", iter, rep.TotalChecked, countScored(stream))
		}
	}
}

// TestProperty_InjectedFillErrorsAreCaught asserts P2: corrupting k reported
// fills produces exactly k mismatches — the validator never silently passes a
// wrong fill.
func TestProperty_InjectedFillErrorsAreCaught(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for iter := range propIterations {
		cid := fmt.Sprintf("c%d", iter)
		stream := genStream(rng, cid, rng.Intn(40)+5)

		// Perturb a random subset of scored events to a deliberately wrong fill.
		var corrupted int64
		for i := range stream {
			if stream[i].scored && rng.Intn(3) == 0 {
				stream[i].ev.FilledQuantity = stream[i].oracleFill + int64(rng.Intn(5)+1)
				corrupted++
			}
		}
		if corrupted == 0 {
			continue // nothing perturbed this round; covered by P1
		}

		v := NewValidator()
		for _, s := range stream {
			v.Process(s.ev)
		}
		rep, _ := v.Report(cid)
		if rep.Mismatches != corrupted {
			t.Fatalf("iter %d: mismatches=%d, want exactly %d corrupted fills (rep=%+v)",
				iter, rep.Mismatches, corrupted, rep)
		}
		if rep.Correctness >= 1.0 {
			t.Fatalf("iter %d: correctness should drop below 1.0 with %d corrupted fills, got %v",
				iter, corrupted, rep.Correctness)
		}
	}
}

// TestProperty_ScoringIsDeterministic asserts P3: identical input → identical
// report, run twice.
func TestProperty_ScoringIsDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for iter := range propIterations {
		cid := fmt.Sprintf("c%d", iter)
		stream := genStream(rng, cid, rng.Intn(40)+1)

		run := func() Report {
			v := NewValidator()
			for _, s := range stream {
				v.Process(s.ev)
			}
			rep, _ := v.Report(cid)
			return rep
		}
		a, b := run(), run()
		if a != b {
			t.Fatalf("iter %d: non-deterministic scoring: %+v != %+v", iter, a, b)
		}
	}
}
