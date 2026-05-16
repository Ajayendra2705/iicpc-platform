package replay_test

import (
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/replay"
)

func ord(id string, side replay.Side, price float64, qty int64, ts int64) replay.Order {
	return replay.Order{ID: id, Side: side, Price: price, Qty: qty, TsNs: ts}
}

func TestBuyNoMatchRestsOnBook(t *testing.T) {
	b := replay.New()
	filled := b.Place(ord("b1", replay.Buy, 100, 5, 1))
	if filled != 0 {
		t.Errorf("filled: got %d want 0", filled)
	}
	p, ok := b.BestBid()
	if !ok || p != 100 {
		t.Errorf("bid: got %v ok=%v", p, ok)
	}
}

func TestBuyFullyFilledAgainstAsk(t *testing.T) {
	b := replay.New()
	b.Place(ord("s1", replay.Sell, 99, 10, 1))
	filled := b.Place(ord("b1", replay.Buy, 100, 5, 2))
	if filled != 5 {
		t.Errorf("filled: got %d want 5", filled)
	}
	if _, ok := b.BestBid(); ok {
		t.Error("no bid should rest after full fill")
	}
}

func TestBuyPartiallyFilled(t *testing.T) {
	b := replay.New()
	b.Place(ord("s1", replay.Sell, 99, 3, 1))
	filled := b.Place(ord("b1", replay.Buy, 100, 10, 2))
	if filled != 3 {
		t.Errorf("filled: got %d want 3", filled)
	}
	if p, ok := b.BestBid(); !ok || p != 100 {
		t.Errorf("remainder should rest: bid=%v ok=%v", p, ok)
	}
}

func TestBuyOnlyMatchesAtOrBelowLimit(t *testing.T) {
	b := replay.New()
	b.Place(ord("s1", replay.Sell, 101, 5, 1))
	filled := b.Place(ord("b1", replay.Buy, 100, 5, 2))
	if filled != 0 {
		t.Errorf("filled: got %d want 0 (ask above limit)", filled)
	}
}

func TestPriceTimePriority(t *testing.T) {
	b := replay.New()
	b.Place(ord("s1", replay.Sell, 100, 5, 1)) // earlier
	b.Place(ord("s2", replay.Sell, 100, 5, 2)) // later, same price
	filled := b.Place(ord("b1", replay.Buy, 100, 5, 3))
	if filled != 5 {
		t.Errorf("filled: got %d want 5", filled)
	}
	// s1 should be fully consumed; s2 remains
	if p, ok := b.BestAsk(); !ok || p != 100 {
		t.Errorf("s2 should still be top ask: got %v ok=%v", p, ok)
	}
	// cancel s2 and verify book empties
	if !b.Cancel("s2") {
		t.Error("cancel s2 returned false")
	}
	if _, ok := b.BestAsk(); ok {
		t.Error("ask side should be empty after cancel")
	}
}

func TestBestBidIsHighest(t *testing.T) {
	b := replay.New()
	b.Place(ord("b1", replay.Buy, 100, 5, 1))
	b.Place(ord("b2", replay.Buy, 102, 5, 2))
	b.Place(ord("b3", replay.Buy, 101, 5, 3))
	p, _ := b.BestBid()
	if p != 102 {
		t.Errorf("BestBid: got %v want 102", p)
	}
}

func TestBestAskIsLowest(t *testing.T) {
	b := replay.New()
	b.Place(ord("s1", replay.Sell, 100, 5, 1))
	b.Place(ord("s2", replay.Sell, 98, 5, 2))
	b.Place(ord("s3", replay.Sell, 99, 5, 3))
	p, _ := b.BestAsk()
	if p != 98 {
		t.Errorf("BestAsk: got %v want 98", p)
	}
}

func TestCancelUnknownReturnsFalse(t *testing.T) {
	b := replay.New()
	if b.Cancel("nope") {
		t.Error("Cancel of unknown id should return false")
	}
}

func TestSellWalksMultipleBidLevels(t *testing.T) {
	b := replay.New()
	b.Place(ord("b1", replay.Buy, 100, 3, 1))
	b.Place(ord("b2", replay.Buy, 99, 4, 2))
	b.Place(ord("b3", replay.Buy, 98, 10, 3))
	filled := b.Place(ord("s1", replay.Sell, 99, 10, 4))
	// should fill 3 @ 100 (b1) + 4 @ 99 (b2) = 7; b3 @ 98 ignored (below limit 99)
	if filled != 7 {
		t.Errorf("filled: got %d want 7", filled)
	}
}
