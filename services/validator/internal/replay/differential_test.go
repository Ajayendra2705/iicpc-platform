package replay

import (
	"fmt"
	"math/rand"
	"testing"
)

// Differential test for the validator's matching engine.
//
// The property tests in validator_property_test.go pin the validator's wiring
// but use the same Book as their oracle, so they cannot catch a bug in the
// matcher itself. This file closes that gap (the "differential testing against a
// second independent engine" item in IDEAS.md): it drives identical random order
// streams through the production Book AND through naiveBook — a deliberately
// simple, independently-written price-time-priority matcher — and asserts they
// agree on every fill and on the resulting book state, over thousands of cases.
//
// This is the system's core fairness invariant in microcosm: the engine that
// scores contestants must match an independent reference implementation, or a
// correct contestant could be marked wrong.

// naiveOrder is a resting order in the brute-force oracle.
type naiveOrder struct {
	id    string
	side  Side
	price float64
	qty   int64
	ts    int64
}

// naiveBook is an independent, obviously-correct price-time-priority matcher.
// It favours clarity over speed: the best opposite order is found by a linear
// scan each step, and resting orders live in unsorted slices. Time priority
// uses ts (earlier wins) at equal price.
type naiveBook struct {
	bids []naiveOrder // resting buys
	asks []naiveOrder // resting sells
}

// bestAskIdx returns the index of the ask to match first: lowest price, then
// earliest ts. -1 if none.
func (n *naiveBook) bestAskIdx() int {
	best := -1
	for i, o := range n.asks {
		if best == -1 || o.price < n.asks[best].price ||
			(o.price == n.asks[best].price && o.ts < n.asks[best].ts) {
			best = i
		}
	}
	return best
}

// bestBidIdx returns the index of the bid to match first: highest price, then
// earliest ts. -1 if none.
func (n *naiveBook) bestBidIdx() int {
	best := -1
	for i, o := range n.bids {
		if best == -1 || o.price > n.bids[best].price ||
			(o.price == n.bids[best].price && o.ts < n.bids[best].ts) {
			best = i
		}
	}
	return best
}

func (n *naiveBook) removeAsk(i int) { n.asks = append(n.asks[:i], n.asks[i+1:]...) }
func (n *naiveBook) removeBid(i int) { n.bids = append(n.bids[:i], n.bids[i+1:]...) }

// place matches a limit order against the opposite side and rests the remainder.
func (n *naiveBook) place(o naiveOrder) int64 {
	var filled int64
	if o.side == Buy {
		for o.qty > 0 {
			i := n.bestAskIdx()
			if i == -1 || n.asks[i].price > o.price {
				break
			}
			take := min64(o.qty, n.asks[i].qty)
			filled += take
			o.qty -= take
			n.asks[i].qty -= take
			if n.asks[i].qty == 0 {
				n.removeAsk(i)
			}
		}
		if o.qty > 0 {
			n.bids = append(n.bids, o)
		}
		return filled
	}
	for o.qty > 0 {
		i := n.bestBidIdx()
		if i == -1 || n.bids[i].price < o.price {
			break
		}
		take := min64(o.qty, n.bids[i].qty)
		filled += take
		o.qty -= take
		n.bids[i].qty -= take
		if n.bids[i].qty == 0 {
			n.removeBid(i)
		}
	}
	if o.qty > 0 {
		n.asks = append(n.asks, o)
	}
	return filled
}

// placeMarket matches at any price (IOC); the unfilled remainder is discarded.
func (n *naiveBook) placeMarket(side Side, qty int64) int64 {
	var filled int64
	if side == Buy {
		for qty > 0 {
			i := n.bestAskIdx()
			if i == -1 {
				break
			}
			take := min64(qty, n.asks[i].qty)
			filled += take
			qty -= take
			n.asks[i].qty -= take
			if n.asks[i].qty == 0 {
				n.removeAsk(i)
			}
		}
		return filled
	}
	for qty > 0 {
		i := n.bestBidIdx()
		if i == -1 {
			break
		}
		take := min64(qty, n.bids[i].qty)
		filled += take
		qty -= take
		n.bids[i].qty -= take
		if n.bids[i].qty == 0 {
			n.removeBid(i)
		}
	}
	return filled
}

func (n *naiveBook) cancel(id string) {
	for i, o := range n.bids {
		if o.id == id {
			n.removeBid(i)
			return
		}
	}
	for i, o := range n.asks {
		if o.id == id {
			n.removeAsk(i)
			return
		}
	}
}

func (n *naiveBook) restingQty() (bid, ask int64) {
	for _, o := range n.bids {
		bid += o.qty
	}
	for _, o := range n.asks {
		ask += o.qty
	}
	return bid, ask
}

// restingQty sums the production Book's resting quantity per side (white-box).
func bookRestingQty(b *Book) (bid, ask int64) {
	for _, o := range b.bids {
		bid += o.Qty
	}
	for _, o := range b.asks {
		ask += o.Qty
	}
	return bid, ask
}

// TestProperty_BookMatchesIndependentOracle drives identical random streams
// through the production Book and the naive oracle, asserting identical fills and
// identical book state (best bid/ask + resting quantity) after every operation.
func TestProperty_BookMatchesIndependentOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(20260607))
	for iter := range propIterations {
		book := New()
		oracle := &naiveBook{}
		var liveIDs []string
		n := rng.Intn(60) + 1

		for i := range n {
			id := fmt.Sprintf("o%d", i)
			ts := int64(i + 1)
			switch roll := rng.Intn(10); {
			case roll == 0 && len(liveIDs) > 0:
				victim := liveIDs[rng.Intn(len(liveIDs))]
				book.Cancel(victim)
				oracle.cancel(victim)
			case roll <= 2:
				side := pickSide(rng)
				qty := int64(rng.Intn(10) + 1)
				rs := toReplaySide(side)
				f1 := book.PlaceMarket(rs, qty)
				f2 := oracle.placeMarket(rs, qty)
				if f1 != f2 {
					t.Fatalf("iter %d op %d: market fill mismatch: book=%d oracle=%d", iter, i, f1, f2)
				}
			default:
				side := toReplaySide(pickSide(rng))
				price := 100.0 + float64(rng.Intn(7)-3)
				qty := int64(rng.Intn(10) + 1)
				f1 := book.Place(Order{ID: id, Side: side, Price: price, Qty: qty, TsNs: ts})
				f2 := oracle.place(naiveOrder{id: id, side: side, price: price, qty: qty, ts: ts})
				if f1 != f2 {
					t.Fatalf("iter %d op %d: limit fill mismatch: book=%d oracle=%d (side=%d price=%.0f qty=%d)",
						iter, i, f1, f2, side, price, qty)
				}
				liveIDs = append(liveIDs, id)
			}

			// Book state must agree after every operation.
			bBid, bAsk := bookRestingQty(book)
			oBid, oAsk := oracle.restingQty()
			if bBid != oBid || bAsk != oAsk {
				t.Fatalf("iter %d op %d: resting qty mismatch: book=(%d,%d) oracle=(%d,%d)",
					iter, i, bBid, bAsk, oBid, oAsk)
			}
			assertBestAgree(t, iter, i, book, oracle)
		}
	}
}

func assertBestAgree(t *testing.T, iter, op int, book *Book, oracle *naiveBook) {
	t.Helper()
	bb, bok := book.BestBid()
	var ob float64
	var ook bool
	if i := oracle.bestBidIdx(); i != -1 {
		ob, ook = oracle.bids[i].price, true
	}
	if bok != ook || (bok && bb != ob) {
		t.Fatalf("iter %d op %d: best bid mismatch: book=(%.0f,%v) oracle=(%.0f,%v)", iter, op, bb, bok, ob, ook)
	}
	ba, aok := book.BestAsk()
	var oa float64
	var oaok bool
	if i := oracle.bestAskIdx(); i != -1 {
		oa, oaok = oracle.asks[i].price, true
	}
	if aok != oaok || (aok && ba != oa) {
		t.Fatalf("iter %d op %d: best ask mismatch: book=(%.0f,%v) oracle=(%.0f,%v)", iter, op, ba, aok, oa, oaok)
	}
}
