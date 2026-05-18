package gen

import "fmt"

// Profile picks one of the canonical trader archetypes that the PS asks the
// bot fleet to simulate (PS §"Distributed Load Generator (Bot Fleet)":
// "These bots will simulate diverse market participants"). Each profile is
// a preset Config that biases the underlying generator toward a different
// participant style; the bot-worker selects a profile per-pod via the
// BOT_PROFILE env var so the aggregate fleet shows realistic mix.
type Profile string

const (
	// ProfileMarketMaker quotes both sides tightly around mid, cancels often,
	// rarely takes liquidity. Sigma is narrow; market orders are off.
	ProfileMarketMaker Profile = "market_maker"
	// ProfileAggressiveTaker hits the book with mostly market orders, rarely
	// cancels (would-be IOC orders don't rest), wider price exploration when
	// it does send a limit.
	ProfileAggressiveTaker Profile = "aggressive_taker"
	// ProfileRetail sends small orders sporadically with high price variance
	// (uninformed flow), low cancel rate, modest market-order use.
	ProfileRetail Profile = "retail"
	// ProfileNoise is the default mixed-flow profile (similar to bare
	// Config defaults) — represents the baseline "background" population.
	ProfileNoise Profile = "noise"
)

// PresetConfig returns the canonical Config for a profile. MidPrice and
// PriceSigma still need to come from the operator (since they're market-
// specific); everything else is preset.
func PresetConfig(p Profile, midPrice, priceSigma float64) (Config, error) {
	cfg := Config{MidPrice: midPrice, PriceSigma: priceSigma, MinQty: 1, MaxQty: 10}
	switch p {
	case ProfileMarketMaker:
		// Tight spread, frequent re-quote (high cancel), no market orders.
		cfg.PriceSigma = priceSigma * 0.25 // 4× tighter than baseline
		cfg.CancelRatio = 0.85
		cfg.MarketRatio = 0
		cfg.MinQty, cfg.MaxQty = 1, 5
	case ProfileAggressiveTaker:
		// Mostly market orders, almost never cancels (IOC doesn't rest).
		cfg.CancelRatio = 0.05
		cfg.MarketRatio = 0.60
		cfg.MinQty, cfg.MaxQty = 2, 8
	case ProfileRetail:
		// Wide-price uninformed flow, low cancel, occasional market order.
		cfg.PriceSigma = priceSigma * 2.5 // 2.5× wider than baseline
		cfg.CancelRatio = 0.15
		cfg.MarketRatio = 0.10
		cfg.MinQty, cfg.MaxQty = 1, 3
	case ProfileNoise, "":
		// Mixed background flow — close to bare defaults.
		cfg.CancelRatio = 0.50
		cfg.MarketRatio = 0.10
		cfg.MinQty, cfg.MaxQty = 1, 10
	default:
		return Config{}, fmt.Errorf("unknown bot profile %q (want market_maker|aggressive_taker|retail|noise)", p)
	}
	return cfg, nil
}

// AllProfiles is the canonical list of supported profiles, useful for
// validation and for the bot-coordinator to round-robin across them.
func AllProfiles() []Profile {
	return []Profile{
		ProfileMarketMaker,
		ProfileAggressiveTaker,
		ProfileRetail,
		ProfileNoise,
	}
}
