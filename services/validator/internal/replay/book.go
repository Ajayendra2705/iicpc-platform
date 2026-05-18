// Package replay provides a minimal price-time priority orderbook used to
// validate contestant orderbook behavior. Optimised for correctness, not
// throughput — sorted-slice insertion is O(N) but the validator is offline.
package replay

import (
	"sort"
)

// Side is the buy/sell direction of an order.
type Side int

const (
	Buy Side = iota
	Sell
)

// Order is a resting limit order on the book.
type Order struct {
	ID    string
	Side  Side
	Price float64
	Qty   int64
	TsNs  int64 // monotonic timestamp for time priority
}

// Book holds bids and asks for one symbol. Not safe for concurrent use; the
// validator pins one Book per contestant and processes events serially.
type Book struct {
	bids []Order // sorted desc by price, then asc by ts (best bid first)
	asks []Order // sorted asc by price, then asc by ts (best ask first)
}

func New() *Book { return &Book{} }

// Place attempts to match an incoming order at limit price. Returns the
// quantity filled. Any unfilled remainder rests on the book.
func (b *Book) Place(o Order) int64 {
	switch o.Side {
	case Buy:
		filled := b.matchBuy(&o)
		if o.Qty > 0 {
			b.insertBid(o)
		}
		return filled
	case Sell:
		filled := b.matchSell(&o)
		if o.Qty > 0 {
			b.insertAsk(o)
		}
		return filled
	}
	return 0
}

// PlaceMarket matches an incoming market order against the opposite book at
// any price (IOC). The unfilled remainder is discarded — market orders do not
// rest. Returns the quantity filled. Used by the validator to score
// contestant fills against canonical market-order semantics.
func (b *Book) PlaceMarket(side Side, qty int64) int64 {
	switch side {
	case Buy:
		var filled int64
		for len(b.asks) > 0 && qty > 0 {
			ask := &b.asks[0]
			take := min64(qty, ask.Qty)
			filled += take
			qty -= take
			ask.Qty -= take
			if ask.Qty == 0 {
				b.asks = b.asks[1:]
			}
		}
		return filled
	case Sell:
		var filled int64
		for len(b.bids) > 0 && qty > 0 {
			bid := &b.bids[0]
			take := min64(qty, bid.Qty)
			filled += take
			qty -= take
			bid.Qty -= take
			if bid.Qty == 0 {
				b.bids = b.bids[1:]
			}
		}
		return filled
	}
	return 0
}

// Cancel removes the order with the given id from the book. Returns true
// if found and removed.
func (b *Book) Cancel(id string) bool {
	for i, o := range b.bids {
		if o.ID == id {
			b.bids = append(b.bids[:i], b.bids[i+1:]...)
			return true
		}
	}
	for i, o := range b.asks {
		if o.ID == id {
			b.asks = append(b.asks[:i], b.asks[i+1:]...)
			return true
		}
	}
	return false
}

// BestBid returns the highest resting bid, or 0/false if none.
func (b *Book) BestBid() (float64, bool) {
	if len(b.bids) == 0 {
		return 0, false
	}
	return b.bids[0].Price, true
}

// BestAsk returns the lowest resting ask, or 0/false if none.
func (b *Book) BestAsk() (float64, bool) {
	if len(b.asks) == 0 {
		return 0, false
	}
	return b.asks[0].Price, true
}

// matchBuy matches incoming buy against asks; modifies o.Qty in place.
func (b *Book) matchBuy(o *Order) int64 {
	var filled int64
	for len(b.asks) > 0 && o.Qty > 0 {
		ask := &b.asks[0]
		if ask.Price > o.Price {
			break
		}
		take := min64(o.Qty, ask.Qty)
		filled += take
		o.Qty -= take
		ask.Qty -= take
		if ask.Qty == 0 {
			b.asks = b.asks[1:]
		}
	}
	return filled
}

func (b *Book) matchSell(o *Order) int64 {
	var filled int64
	for len(b.bids) > 0 && o.Qty > 0 {
		bid := &b.bids[0]
		if bid.Price < o.Price {
			break
		}
		take := min64(o.Qty, bid.Qty)
		filled += take
		o.Qty -= take
		bid.Qty -= take
		if bid.Qty == 0 {
			b.bids = b.bids[1:]
		}
	}
	return filled
}

func (b *Book) insertBid(o Order) {
	idx := sort.Search(len(b.bids), func(i int) bool {
		if b.bids[i].Price != o.Price {
			return b.bids[i].Price < o.Price // higher prices come first
		}
		return b.bids[i].TsNs > o.TsNs // earlier ts comes first at same price
	})
	b.bids = append(b.bids, Order{})
	copy(b.bids[idx+1:], b.bids[idx:])
	b.bids[idx] = o
}

func (b *Book) insertAsk(o Order) {
	idx := sort.Search(len(b.asks), func(i int) bool {
		if b.asks[i].Price != o.Price {
			return b.asks[i].Price > o.Price // lower prices come first
		}
		return b.asks[i].TsNs > o.TsNs // earlier ts comes first at same price
	})
	b.asks = append(b.asks, Order{})
	copy(b.asks[idx+1:], b.asks[idx:])
	b.asks[idx] = o
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
