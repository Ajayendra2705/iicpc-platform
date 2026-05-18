package gen_test

import (
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
)

func TestPresetConfigKnownProfiles(t *testing.T) {
	for _, p := range gen.AllProfiles() {
		cfg, err := gen.PresetConfig(p, 100, 1.0)
		if err != nil {
			t.Errorf("profile %q: unexpected error %v", p, err)
			continue
		}
		if cfg.MidPrice != 100 {
			t.Errorf("profile %q: MidPrice not preserved: %v", p, cfg.MidPrice)
		}
	}
}

func TestPresetConfigRejectsUnknown(t *testing.T) {
	_, err := gen.PresetConfig("bogus", 100, 1.0)
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestProfileBehaviourDiffers(t *testing.T) {
	// Market maker should produce 0 market orders; aggressive taker should
	// produce a lot. This is the load-bearing assertion — the whole point
	// of profiles is that the aggregate fleet shows diverse behaviour.
	mmCfg, _ := gen.PresetConfig(gen.ProfileMarketMaker, 100, 1.0)
	atCfg, _ := gen.PresetConfig(gen.ProfileAggressiveTaker, 100, 1.0)

	mm := gen.New(mmCfg)
	at := gen.New(atCfg)
	const n = 5000
	mmMarkets, atMarkets := 0, 0
	for range n {
		if mm.Next().Kind == gen.Market {
			mmMarkets++
		}
		if at.Next().Kind == gen.Market {
			atMarkets++
		}
	}
	if mmMarkets != 0 {
		t.Errorf("market_maker produced %d market orders (expected 0)", mmMarkets)
	}
	// aggressive_taker should be near 60% — give wide margin for randomness.
	atRatio := float64(atMarkets) / n
	if atRatio < 0.55 || atRatio > 0.65 {
		t.Errorf("aggressive_taker market ratio %.3f outside [0.55, 0.65]", atRatio)
	}
}

func TestProfileEmptyDefaultsToNoise(t *testing.T) {
	cfg, err := gen.PresetConfig("", 100, 1.0)
	if err != nil {
		t.Fatalf("empty profile: %v", err)
	}
	noise, _ := gen.PresetConfig(gen.ProfileNoise, 100, 1.0)
	if cfg.CancelRatio != noise.CancelRatio || cfg.MarketRatio != noise.MarketRatio {
		t.Error("empty profile should equal noise profile")
	}
}
