// Package score computes the composite leaderboard score from aggregator
// latency/TPS metrics and validator correctness, with crash/timeout penalties.
//
//	base  = 1000 × ( w_lat·latency_norm + w_tps·tps_norm + w_corr·correctness )
//	final = max(0, base − crashes·CrashPenalty − timeouts·TimeoutPenalty)
//
// where:
//
//	latency_norm = clamp01(1 − P99 / MaxP99)   // lower P99 → higher norm
//	tps_norm     = clamp01(TPS / TargetTPS)    // higher TPS → higher norm
//	correctness  = validator's fraction in [0,1]
package score

import "math"

// Config is the scoring policy. Defaults match the hackathon brief.
type Config struct {
	MaxP99Ns          int64   // P99 above this → latency_norm = 0
	TargetTPS         float64 // TPS at or above this → tps_norm = 1
	LatencyWeight     float64 // default 0.4
	TPSWeight         float64 // default 0.3
	CorrectnessWeight float64 // default 0.3
	CrashPenalty      int64   // points subtracted per crash, default 10_000
	TimeoutPenalty    int64   // points subtracted per timeout event, default 1_000
	Scale             float64 // multiplier that turns 0-1 normalized into a points value, default 1000
}

// DefaultConfig returns the hackathon-brief weights and penalties.
func DefaultConfig() Config {
	return Config{
		MaxP99Ns:          100_000_000, // 100 ms
		TargetTPS:         10_000,
		LatencyWeight:     0.4,
		TPSWeight:         0.3,
		CorrectnessWeight: 0.3,
		CrashPenalty:      10_000,
		TimeoutPenalty:    1_000,
		Scale:             1_000,
	}
}

// Inputs captures one snapshot of a contestant's measured performance.
type Inputs struct {
	P99Ns       int64
	TPS         float64
	Correctness float64 // 0–1
	Crashes     int64
	Timeouts    int64
}

// Result is the per-contestant score breakdown; FinalScore is what feeds
// the Redis ZSET in D20.
type Result struct {
	LatencyNorm float64 `json:"latency_norm"`
	TPSNorm     float64 `json:"tps_norm"`
	Correctness float64 `json:"correctness"`
	BaseScore   float64 `json:"base_score"`
	Penalty     int64   `json:"penalty"`
	FinalScore  int64   `json:"final_score"`
}

// Calculator applies a Config to compute Results.
type Calculator struct{ cfg Config }

func New(cfg Config) *Calculator {
	if cfg.MaxP99Ns <= 0 {
		cfg = DefaultConfig()
	}
	return &Calculator{cfg: cfg}
}

// Compute produces a Result for the given inputs.
func (c *Calculator) Compute(in Inputs) Result {
	latNorm := clamp01(1.0 - float64(in.P99Ns)/float64(c.cfg.MaxP99Ns))
	tpsNorm := clamp01(in.TPS / c.cfg.TargetTPS)
	corr := clamp01(in.Correctness)

	base := c.cfg.Scale * (c.cfg.LatencyWeight*latNorm + c.cfg.TPSWeight*tpsNorm + c.cfg.CorrectnessWeight*corr)
	penalty := in.Crashes*c.cfg.CrashPenalty + in.Timeouts*c.cfg.TimeoutPenalty

	final := max(int64(math.Round(base))-penalty, 0)
	return Result{
		LatencyNorm: latNorm,
		TPSNorm:     tpsNorm,
		Correctness: corr,
		BaseScore:   base,
		Penalty:     penalty,
		FinalScore:  final,
	}
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
