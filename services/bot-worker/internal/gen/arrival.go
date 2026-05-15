package gen

import (
	"math"
	"math/rand"
	"time"
)

// ArrivalMode controls inter-arrival time distribution.
type ArrivalMode string

const (
	// ArrivalUniform produces a fixed interval between orders (default).
	ArrivalUniform ArrivalMode = "uniform"
	// ArrivalPoisson produces exponentially-distributed inter-arrival times,
	// yielding a true Poisson process at the configured mean rate.
	ArrivalPoisson ArrivalMode = "poisson"
)

// Arrivals generates inter-arrival durations for the bot worker.
// Mean rate is 1/interval regardless of mode.
type Arrivals struct {
	mode     ArrivalMode
	interval time.Duration // mean inter-arrival time (= 1/rps)
	rng      *rand.Rand
}

// NewArrivals creates an Arrivals generator for the given mode and rate.
// rps ≤ 0 is clamped to 1.
func NewArrivals(mode ArrivalMode, rps float64) *Arrivals {
	if rps <= 0 {
		rps = 1
	}
	return &Arrivals{
		mode:     mode,
		interval: time.Duration(float64(time.Second) / rps),
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec
	}
}

// Next returns the duration to wait before sending the next order.
// For Poisson mode the duration is exponentially distributed so the
// arrival process is memoryless (inter-arrival CV = 1).
func (a *Arrivals) Next() time.Duration {
	switch a.mode {
	case ArrivalPoisson:
		// Inverse-CDF of Exp(λ): T = -ln(U) / λ = -ln(U) * E[T]
		v := time.Duration(-math.Log(a.rng.Float64()) * float64(a.interval))
		if v <= 0 {
			return a.interval // guard: rng == 0 → -ln(0) = +Inf
		}
		return v
	default: // ArrivalUniform
		return a.interval
	}
}
