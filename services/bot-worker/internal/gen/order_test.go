package gen_test

import (
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
)

func TestNextValid(t *testing.T) {
	// MarketRatio: 0 → all limit orders (so o.Price must be >0)
	g := gen.New(gen.Config{MidPrice: 100, PriceSigma: 1, MinQty: 1, MaxQty: 10, CancelRatio: 0.7})
	for range 1000 {
		o := g.Next()
		if o.Kind != gen.Limit {
			t.Fatalf("expected Limit (MarketRatio=0), got %s", o.Kind)
		}
		if o.Price <= 0 {
			t.Fatalf("non-positive price: %f", o.Price)
		}
		if o.Qty < 1 || o.Qty > 10 {
			t.Fatalf("qty out of range: %d", o.Qty)
		}
		if o.Side != gen.Buy && o.Side != gen.Sell {
			t.Fatalf("invalid side: %s", o.Side)
		}
	}
}

func TestMarketRatioDistribution(t *testing.T) {
	g := gen.New(gen.Config{MidPrice: 100, PriceSigma: 1, MarketRatio: 0.20})
	const n = 10_000
	markets := 0
	for range n {
		o := g.Next()
		if o.Kind == gen.Market {
			markets++
			if o.Price != 0 {
				t.Fatalf("market order with non-zero price: %f", o.Price)
			}
		}
	}
	ratio := float64(markets) / n
	if ratio < 0.17 || ratio > 0.23 {
		t.Fatalf("market ratio %.3f outside [0.17, 0.23]", ratio)
	}
}

func TestNextDefaults(t *testing.T) {
	g := gen.New(gen.Config{})
	o := g.Next()
	if o.Price <= 0 {
		t.Fatal("default config: non-positive price")
	}
	if o.Qty < 1 {
		t.Fatal("default config: qty < 1")
	}
}

func TestShouldCancelDistribution(t *testing.T) {
	g := gen.New(gen.Config{CancelRatio: 0.70})
	const n = 10_000
	cancels := 0
	for range n {
		if g.ShouldCancel() {
			cancels++
		}
	}
	ratio := float64(cancels) / n
	if ratio < 0.65 || ratio > 0.75 {
		t.Fatalf("cancel ratio %.3f outside [0.65, 0.75]", ratio)
	}
}

func TestPriceCentredOnMid(t *testing.T) {
	g := gen.New(gen.Config{MidPrice: 500, PriceSigma: 0.001})
	for range 100 {
		o := g.Next()
		if o.Price < 499 || o.Price > 501 {
			t.Fatalf("price %.2f far from mid 500", o.Price)
		}
	}
}
