package engine

import (
	"container/heap"
	"fmt"
	"sync"
	"time"
)

type Side int

const (
	Buy  Side = iota
	Sell Side = iota
)

type Order struct {
	ID        string
	Side      Side
	Price     float64
	Qty       int64
	Remaining int64
	At        time.Time
}

type Fill struct {
	BuyOrderID  string
	SellOrderID string
	Price       float64
	Qty         int64
}

type Level struct {
	Price float64
	Qty   int64
}

// bidEntry: max-price, min-time priority
type bidEntry struct {
	o   *Order
	idx int
}

type bidHeap []*bidEntry

func (h bidHeap) Len() int      { return len(h) }
func (h bidHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i]; h[i].idx = i; h[j].idx = j }
func (h bidHeap) Less(i, j int) bool {
	if h[i].o.Price != h[j].o.Price {
		return h[i].o.Price > h[j].o.Price // max price first
	}
	return h[i].o.At.Before(h[j].o.At) // earlier time first
}
func (h *bidHeap) Push(x any) { e := x.(*bidEntry); e.idx = len(*h); *h = append(*h, e) }
func (h *bidHeap) Pop() any   { old := *h; n := len(old); e := old[n-1]; *h = old[:n-1]; return e }

// askEntry: min-price, min-time priority
type askEntry struct {
	o   *Order
	idx int
}

type askHeap []*askEntry

func (h askHeap) Len() int      { return len(h) }
func (h askHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i]; h[i].idx = i; h[j].idx = j }
func (h askHeap) Less(i, j int) bool {
	if h[i].o.Price != h[j].o.Price {
		return h[i].o.Price < h[j].o.Price // min price first
	}
	return h[i].o.At.Before(h[j].o.At)
}
func (h *askHeap) Push(x any) { e := x.(*askEntry); e.idx = len(*h); *h = append(*h, e) }
func (h *askHeap) Pop() any   { old := *h; n := len(old); e := old[n-1]; *h = old[:n-1]; return e }

type OrderBook struct {
	mu       sync.Mutex
	bids     bidHeap
	asks     askHeap
	orders   map[string]*Order
	bidIndex map[string]*bidEntry
	askIndex map[string]*askEntry
}

func New() *OrderBook {
	ob := &OrderBook{
		orders:   make(map[string]*Order),
		bidIndex: make(map[string]*bidEntry),
		askIndex: make(map[string]*askEntry),
	}
	heap.Init(&ob.bids)
	heap.Init(&ob.asks)
	return ob
}

func (ob *OrderBook) Place(o *Order) ([]Fill, error) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if _, exists := ob.orders[o.ID]; exists {
		return nil, fmt.Errorf("duplicate order id %s", o.ID)
	}
	if o.Remaining == 0 {
		o.Remaining = o.Qty
	}
	ob.orders[o.ID] = o

	var fills []Fill

	if o.Side == Buy {
		// match against asks
		for o.Remaining > 0 && ob.asks.Len() > 0 {
			// skip cancelled/exhausted tops
			for ob.asks.Len() > 0 && ob.asks[0].o.Remaining == 0 {
				heap.Pop(&ob.asks)
			}
			if ob.asks.Len() == 0 {
				break
			}
			best := ob.asks[0].o
			if best.Price > o.Price {
				break
			}
			qty := min64(o.Remaining, best.Remaining)
			fills = append(fills, Fill{BuyOrderID: o.ID, SellOrderID: best.ID, Price: best.Price, Qty: qty})
			o.Remaining -= qty
			best.Remaining -= qty
			if best.Remaining == 0 {
				heap.Pop(&ob.asks)
				delete(ob.askIndex, best.ID)
			}
		}
		if o.Remaining > 0 {
			e := &bidEntry{o: o}
			heap.Push(&ob.bids, e)
			ob.bidIndex[o.ID] = e
		}
	} else {
		// match against bids
		for o.Remaining > 0 && ob.bids.Len() > 0 {
			for ob.bids.Len() > 0 && ob.bids[0].o.Remaining == 0 {
				heap.Pop(&ob.bids)
			}
			if ob.bids.Len() == 0 {
				break
			}
			best := ob.bids[0].o
			if best.Price < o.Price {
				break
			}
			qty := min64(o.Remaining, best.Remaining)
			fills = append(fills, Fill{BuyOrderID: best.ID, SellOrderID: o.ID, Price: best.Price, Qty: qty})
			o.Remaining -= qty
			best.Remaining -= qty
			if best.Remaining == 0 {
				heap.Pop(&ob.bids)
				delete(ob.bidIndex, best.ID)
			}
		}
		if o.Remaining > 0 {
			e := &askEntry{o: o}
			heap.Push(&ob.asks, e)
			ob.askIndex[o.ID] = e
		}
	}

	return fills, nil
}

func (ob *OrderBook) Cancel(id string) error {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	o, ok := ob.orders[id]
	if !ok {
		return fmt.Errorf("order %s not found", id)
	}
	o.Remaining = 0 // lazy deletion; heap cleanup on next match
	delete(ob.orders, id)
	delete(ob.bidIndex, id)
	delete(ob.askIndex, id)
	return nil
}

type Snapshot struct {
	Bids []Level
	Asks []Level
}

func (ob *OrderBook) Snapshot() Snapshot {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	bidMap := make(map[float64]int64)
	for _, e := range ob.bids {
		if e.o.Remaining > 0 {
			bidMap[e.o.Price] += e.o.Remaining
		}
	}
	askMap := make(map[float64]int64)
	for _, e := range ob.asks {
		if e.o.Remaining > 0 {
			askMap[e.o.Price] += e.o.Remaining
		}
	}

	snap := Snapshot{}
	for p, q := range bidMap {
		snap.Bids = append(snap.Bids, Level{Price: p, Qty: q})
	}
	for p, q := range askMap {
		snap.Asks = append(snap.Asks, Level{Price: p, Qty: q})
	}
	sortLevels(snap.Bids, true)  // desc
	sortLevels(snap.Asks, false) // asc
	return snap
}

func sortLevels(ls []Level, desc bool) {
	for i := 0; i < len(ls); i++ {
		for j := i + 1; j < len(ls); j++ {
			if desc && ls[j].Price > ls[i].Price {
				ls[i], ls[j] = ls[j], ls[i]
			} else if !desc && ls[j].Price < ls[i].Price {
				ls[i], ls[j] = ls[j], ls[i]
			}
		}
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
