package gen

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
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
// Safe for concurrent use across multiple worker goroutines.
type Arrivals struct {
	mu          sync.Mutex
	mode        ArrivalMode
	interval    time.Duration // base mean inter-arrival (1/rps)
	burstIvl    time.Duration // interval / burstMultiplier, set by StartBurst
	maxJitter   time.Duration // uniform jitter added to each sample
	rng         *rand.Rand
	burstActive atomic.Bool
}

// NewArrivals creates an Arrivals generator for the given mode and rate.
// rps ≤ 0 is clamped to 1.
func NewArrivals(mode ArrivalMode, rps float64) *Arrivals {
	if rps <= 0 {
		rps = 1
	}
	ivl := time.Duration(float64(time.Second) / rps)
	return &Arrivals{
		mode:     mode,
		interval: ivl,
		burstIvl: ivl,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec
	}
}

// WithJitter adds uniform random jitter in [0, max) to each inter-arrival sample.
func (a *Arrivals) WithJitter(max time.Duration) *Arrivals {
	a.maxJitter = max
	return a
}

// SetBurstActive forces burst mode on/off; intended for tests.
func (a *Arrivals) SetBurstActive(active bool) {
	a.burstActive.Store(active)
}

// StartBurst periodically activates burst mode for duration every every-period.
// During burst, the effective rate is multiplied by multiplier (e.g. 10×).
// Runs until ctx is cancelled.
func (a *Arrivals) StartBurst(ctx context.Context, every, duration time.Duration, multiplier int) {
	if multiplier <= 1 {
		return
	}
	a.mu.Lock()
	a.burstIvl = a.interval / time.Duration(multiplier)
	if a.burstIvl <= 0 {
		a.burstIvl = time.Millisecond
	}
	a.mu.Unlock()

	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.burstActive.Store(true)
				time.AfterFunc(duration, func() { a.burstActive.Store(false) })
			}
		}
	}()
}

// Next returns the duration to wait before sending the next order.
// Applies burst multiplier, Poisson/uniform mode, and optional jitter.
func (a *Arrivals) Next() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()

	ivl := a.interval
	if a.burstActive.Load() {
		ivl = a.burstIvl
	}

	var base time.Duration
	switch a.mode {
	case ArrivalPoisson:
		// Inverse-CDF of Exp(λ): T = -ln(U) / λ = -ln(U) * E[T]
		v := time.Duration(-math.Log(a.rng.Float64()) * float64(ivl))
		if v <= 0 {
			base = ivl // guard: rng == 0 → -ln(0) = +Inf
		} else {
			base = v
		}
	default: // ArrivalUniform
		base = ivl
	}

	if a.maxJitter > 0 {
		base += time.Duration(a.rng.Int63n(int64(a.maxJitter)))
	}
	return base
}
