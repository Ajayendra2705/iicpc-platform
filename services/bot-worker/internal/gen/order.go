package gen

import (
	"math"
	"math/rand"
	"time"
)

type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

type Order struct {
	Side  Side    `json:"side"`
	Price float64 `json:"price"`
	Qty   int     `json:"qty"`
}

type Config struct {
	MidPrice    float64 // centre of price distribution
	PriceSigma  float64 // std-dev; <=0 → 1.0
	MinQty      int     // <=0 → 1
	MaxQty      int     // <=0 → 10
	CancelRatio float64 // fraction [0,1]; <=0 → 0.70
}

func (c *Config) applyDefaults() {
	if c.MidPrice <= 0 {
		c.MidPrice = 100
	}
	if c.PriceSigma <= 0 {
		c.PriceSigma = 1
	}
	if c.MinQty <= 0 {
		c.MinQty = 1
	}
	if c.MaxQty <= 0 {
		c.MaxQty = 10
	}
	if c.CancelRatio <= 0 {
		c.CancelRatio = 0.70
	}
}

type Generator struct {
	cfg Config
	rng *rand.Rand
}

func New(cfg Config) *Generator {
	cfg.applyDefaults()
	return &Generator{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec
	}
}

func (g *Generator) Next() Order {
	side := Buy
	if g.rng.Intn(2) == 1 {
		side = Sell
	}
	raw := g.cfg.MidPrice + g.rng.NormFloat64()*g.cfg.PriceSigma
	if raw < 0.01 {
		raw = 0.01
	}
	price := math.Round(raw*100) / 100
	qty := g.cfg.MinQty
	if g.cfg.MaxQty > g.cfg.MinQty {
		qty += g.rng.Intn(g.cfg.MaxQty - g.cfg.MinQty + 1)
	}
	return Order{Side: side, Price: price, Qty: qty}
}

func (g *Generator) ShouldCancel() bool {
	return g.rng.Float64() < g.cfg.CancelRatio
}
