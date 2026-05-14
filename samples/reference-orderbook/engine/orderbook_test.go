package engine

import (
	"fmt"
	"testing"
	"time"
)

func ord(id string, side Side, price float64, qty int64) *Order {
	return &Order{ID: id, Side: side, Price: price, Qty: qty, At: time.Now()}
}

func TestBuyMatchesAsk(t *testing.T) {
	ob := New()
	ob.Place(ord("s1", Sell, 100, 10))
	fills, _ := ob.Place(ord("b1", Buy, 100, 10))
	if len(fills) != 1 || fills[0].Qty != 10 {
		t.Fatalf("want 1 fill qty=10, got %v", fills)
	}
}

func TestSellMatchesBid(t *testing.T) {
	ob := New()
	ob.Place(ord("b1", Buy, 100, 5))
	fills, _ := ob.Place(ord("s1", Sell, 100, 5))
	if len(fills) != 1 || fills[0].Qty != 5 {
		t.Fatalf("want 1 fill qty=5, got %v", fills)
	}
}

func TestNoMatchPriceMiss(t *testing.T) {
	ob := New()
	ob.Place(ord("s1", Sell, 101, 10))
	fills, _ := ob.Place(ord("b1", Buy, 100, 10))
	if len(fills) != 0 {
		t.Fatalf("want 0 fills, got %v", fills)
	}
}

func TestPricePriority(t *testing.T) {
	ob := New()
	// two asks; best price should match first
	ob.Place(ord("s2", Sell, 102, 5))
	ob.Place(ord("s1", Sell, 100, 5))
	fills, _ := ob.Place(ord("b1", Buy, 105, 5))
	if len(fills) != 1 || fills[0].SellOrderID != "s1" {
		t.Fatalf("want s1 matched first, got %v", fills)
	}
}

func TestPartialFill(t *testing.T) {
	ob := New()
	ob.Place(ord("s1", Sell, 100, 3))
	fills, _ := ob.Place(ord("b1", Buy, 100, 10))
	if len(fills) != 1 || fills[0].Qty != 3 {
		t.Fatalf("want partial fill qty=3, got %v", fills)
	}
	snap := ob.Snapshot()
	if len(snap.Bids) != 1 || snap.Bids[0].Qty != 7 {
		t.Fatalf("want remaining bid qty=7, got %v", snap.Bids)
	}
}

func TestCancel(t *testing.T) {
	ob := New()
	ob.Place(ord("s1", Sell, 100, 10))
	ob.Cancel("s1")
	fills, _ := ob.Place(ord("b1", Buy, 100, 10))
	if len(fills) != 0 {
		t.Fatalf("cancelled order should not match, got %v", fills)
	}
}

func TestCancelNotFound(t *testing.T) {
	ob := New()
	err := ob.Cancel("nonexistent")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestCancelEagerlyRemovesFromHeap(t *testing.T) {
	ob := New()
	for i := range 100 {
		ob.Place(ord(fmt.Sprintf("b%d", i), Buy, float64(100+i), 1))
	}
	for i := range 100 {
		ob.Cancel(fmt.Sprintf("b%d", i))
	}
	ob.mu.Lock()
	heapLen := ob.bids.Len()
	ob.mu.Unlock()
	if heapLen != 0 {
		t.Fatalf("heap should be empty after all cancels, got len=%d", heapLen)
	}
}

func TestSnapshot(t *testing.T) {
	ob := New()
	ob.Place(ord("b1", Buy, 100, 5))
	ob.Place(ord("b2", Buy, 99, 3))
	ob.Place(ord("s1", Sell, 101, 4))
	snap := ob.Snapshot()
	if len(snap.Bids) != 2 {
		t.Fatalf("want 2 bid levels, got %d", len(snap.Bids))
	}
	if snap.Bids[0].Price != 100 {
		t.Fatalf("bids not sorted desc, top=%v", snap.Bids[0])
	}
	if len(snap.Asks) != 1 || snap.Asks[0].Price != 101 {
		t.Fatalf("want 1 ask level at 101, got %v", snap.Asks)
	}
}
