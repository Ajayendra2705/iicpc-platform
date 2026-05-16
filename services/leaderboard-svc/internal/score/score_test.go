package score_test

import (
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/score"
)

func TestPerfectInputsTopScore(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{
		P99Ns:       1_000,  // 1µs — well under MaxP99Ns
		TPS:         50_000, // well over TargetTPS
		Correctness: 1.0,
	})
	if r.LatencyNorm <= 0.99 {
		t.Errorf("latency_norm: got %v, expected ~1.0", r.LatencyNorm)
	}
	if r.TPSNorm != 1.0 {
		t.Errorf("tps_norm: got %v, want 1.0 (clamped)", r.TPSNorm)
	}
	if r.FinalScore < 990 {
		t.Errorf("final_score: got %d, expected ≥ 990 (perfect ≈ 1000)", r.FinalScore)
	}
}

func TestHighLatencyKillsLatencyNorm(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{
		P99Ns:       500_000_000, // 500ms, way over 100ms cap
		TPS:         10_000,
		Correctness: 1.0,
	})
	if r.LatencyNorm != 0 {
		t.Errorf("latency_norm: got %v, want 0 (clamped to floor)", r.LatencyNorm)
	}
	// base = 1000 × (0.4·0 + 0.3·1 + 0.3·1) = 600
	if r.FinalScore != 600 {
		t.Errorf("final_score: got %d, want 600", r.FinalScore)
	}
}

func TestZeroTPS(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{
		P99Ns:       1_000,
		TPS:         0,
		Correctness: 1.0,
	})
	if r.TPSNorm != 0 {
		t.Errorf("tps_norm: got %v, want 0", r.TPSNorm)
	}
}

func TestZeroCorrectness(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{
		P99Ns:       1_000,
		TPS:         10_000,
		Correctness: 0,
	})
	// base = 1000 × (0.4·1 + 0.3·1 + 0.3·0) = 700
	if r.FinalScore != 700 {
		t.Errorf("final_score: got %d, want 700", r.FinalScore)
	}
}

func TestCrashPenaltyApplied(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{
		P99Ns: 1_000, TPS: 10_000, Correctness: 1.0,
		Crashes: 1,
	})
	if r.Penalty != 10_000 {
		t.Errorf("penalty: got %d, want 10000", r.Penalty)
	}
	if r.FinalScore != 0 {
		t.Errorf("final_score: got %d, want 0 (penalty exceeds base)", r.FinalScore)
	}
}

func TestTimeoutPenaltyApplied(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{
		P99Ns: 1_000, TPS: 10_000, Correctness: 1.0,
		Timeouts: 100, // 100 × -1000 = -100_000
	})
	if r.Penalty != 100_000 {
		t.Errorf("penalty: got %d, want 100000", r.Penalty)
	}
	if r.FinalScore != 0 {
		t.Errorf("final_score: got %d, want 0", r.FinalScore)
	}
}

func TestFinalScoreNeverNegative(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{Crashes: 1000})
	if r.FinalScore < 0 {
		t.Errorf("final_score: got %d (must clamp at 0)", r.FinalScore)
	}
}

func TestCorrectnessOver1Clamped(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{P99Ns: 1, TPS: 10_000, Correctness: 5.0})
	if r.Correctness != 1.0 {
		t.Errorf("correctness clamp: got %v, want 1.0", r.Correctness)
	}
}

func TestCorrectnessNegativeClamped(t *testing.T) {
	c := score.New(score.DefaultConfig())
	r := c.Compute(score.Inputs{P99Ns: 1, TPS: 10_000, Correctness: -0.5})
	if r.Correctness != 0 {
		t.Errorf("correctness clamp: got %v, want 0", r.Correctness)
	}
}

func TestDefaultConfigPassedAsZero(t *testing.T) {
	// passing zero-value config should fall back to defaults
	c := score.New(score.Config{})
	r := c.Compute(score.Inputs{P99Ns: 1_000, TPS: 50_000, Correctness: 1.0})
	if r.FinalScore < 990 {
		t.Errorf("zero-config fallback to defaults broken: final_score=%d", r.FinalScore)
	}
}

func TestCustomWeightsHonored(t *testing.T) {
	cfg := score.DefaultConfig()
	cfg.LatencyWeight = 1.0
	cfg.TPSWeight = 0
	cfg.CorrectnessWeight = 0
	c := score.New(cfg)
	r := c.Compute(score.Inputs{P99Ns: 1_000, TPS: 0, Correctness: 0})
	// only latency weight matters, so final_score ≈ Scale × latNorm ≈ 1000
	if r.FinalScore < 990 {
		t.Errorf("custom weights: got %d, want ≥ 990", r.FinalScore)
	}
}
