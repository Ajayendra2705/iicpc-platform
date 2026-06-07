package engine

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// Differential test for the reference orderbook — the heap-based engine that
// contestants build against (the "smoke contestant"). It drives identical random
// streams through OrderBook AND through an independently-written brute-force
// matcher (naiveBook) and asserts identical fills and book state.
//
// The validator's matcher is proven against the same kind of independent oracle
// (services/validator/internal/replay/differential_test.go). Both engines
// matching their oracle is what guarantees the engine contestants build against
// and the engine that scores them agree — the platform's core fairness invariant
// — without coupling the two modules.

type naiveOrder struct {
	id    string
	side  Side
	price float64
	qty   int64
	ts    int64
}

// naiveBook is an obviously-correct price-time-priority matcher: best opposite
// order found by linear scan, earlier ts wins at equal price.
type naiveBook struct {
	bids []naiveOrder
	asks []naiveOrder
}

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

func (n *naiveBook) placeMarket(o naiveOrder) int64 {
	var filled int64
	if o.side == Buy {
		for o.qty > 0 {
			i := n.bestAskIdx()
			if i == -1 {
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
		return filled
	}
	for o.qty > 0 {
		i := n.bestBidIdx()
		if i == -1 {
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

func fillQty(fills []Fill) int64 {
	var q int64
	for _, f := range fills {
		q += f.Qty
	}
	return q
}

func snapshotQty(s Snapshot) (bid, ask int64) {
	for _, l := range s.Bids {
		bid += l.Qty
	}
	for _, l := range s.Asks {
		ask += l.Qty
	}
	return bid, ask
}

// TestDifferential_OrderBookMatchesIndependentOracle drives identical random
// streams through the heap OrderBook and the naive oracle, asserting equal fills
// and equal resting quantity after every operation.
func TestDifferential_OrderBookMatchesIndependentOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(20260607))
	const iterations = 2000

	for iter := range iterations {
		ob := New()
		oracle := &naiveBook{}
		var liveIDs []string
		n := rng.Intn(60) + 1

		for i := range n {
			id := fmt.Sprintf("it%d-o%d", iter, i)
			ts := int64(i + 1)
			side := Buy
			if rng.Intn(2) == 1 {
				side = Sell
			}
			switch roll := rng.Intn(10); {
			case roll == 0 && len(liveIDs) > 0:
				victim := liveIDs[rng.Intn(len(liveIDs))]
				_ = ob.Cancel(victim)
				oracle.cancel(victim)
			case roll <= 2:
				qty := int64(rng.Intn(10) + 1)
				fills, err := ob.PlaceMarket(&Order{ID: id, Side: side, Qty: qty, At: time.Unix(0, ts)})
				if err != nil {
					t.Fatalf("iter %d op %d: PlaceMarket err: %v", iter, i, err)
				}
				f1 := fillQty(fills)
				f2 := oracle.placeMarket(naiveOrder{id: id, side: side, qty: qty, ts: ts})
				if f1 != f2 {
					t.Fatalf("iter %d op %d: market fill mismatch: ob=%d oracle=%d", iter, i, f1, f2)
				}
			default:
				price := 100.0 + float64(rng.Intn(7)-3)
				qty := int64(rng.Intn(10) + 1)
				fills, err := ob.Place(&Order{ID: id, Side: side, Price: price, Qty: qty, At: time.Unix(0, ts)})
				if err != nil {
					t.Fatalf("iter %d op %d: Place err: %v", iter, i, err)
				}
				f1 := fillQty(fills)
				f2 := oracle.place(naiveOrder{id: id, side: side, price: price, qty: qty, ts: ts})
				if f1 != f2 {
					t.Fatalf("iter %d op %d: limit fill mismatch: ob=%d oracle=%d (side=%d price=%.0f qty=%d)",
						iter, i, f1, f2, side, price, qty)
				}
				liveIDs = append(liveIDs, id)
			}

			obBid, obAsk := snapshotQty(ob.Snapshot())
			oBid, oAsk := oracle.restingQty()
			if obBid != oBid || obAsk != oAsk {
				t.Fatalf("iter %d op %d: resting qty mismatch: ob=(%d,%d) oracle=(%d,%d)",
					iter, i, obBid, obAsk, oBid, oAsk)
			}
		}
	}
}
