package gen_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
)

func TestJitterInRange(t *testing.T) {
	const maxJitter = 5 * time.Millisecond
	a := gen.NewArrivals(gen.ArrivalUniform, 100).WithJitter(maxJitter) // 10ms base
	base := 10 * time.Millisecond
	for range 500 {
		got := a.Next()
		if got < base || got >= base+maxJitter+time.Millisecond {
			t.Fatalf("jitter out of range: got %v want [%v, %v)", got, base, base+maxJitter)
		}
	}
}

func TestJitterZeroNoEffect(t *testing.T) {
	a := gen.NewArrivals(gen.ArrivalUniform, 100) // no jitter
	want := 10 * time.Millisecond
	for range 100 {
		if got := a.Next(); got != want {
			t.Fatalf("zero jitter changed interval: got %v want %v", got, want)
		}
	}
}

func TestBurstActiveReducesInterval(t *testing.T) {
	const rps = 10.0 // 100ms base interval
	const mult = 10  // burst → 10ms interval
	a := gen.NewArrivals(gen.ArrivalUniform, rps)
	a.StartBurst(context.Background(), time.Hour, time.Hour, mult) // configure burst multiplier without triggering
	a.SetBurstActive(true)
	for range 50 {
		got := a.Next()
		if got >= 100*time.Millisecond {
			t.Fatalf("burst not reducing interval: got %v want < 100ms", got)
		}
	}
}

func TestBurstInactiveNormal(t *testing.T) {
	a := gen.NewArrivals(gen.ArrivalUniform, 10) // 100ms
	a.StartBurst(context.Background(), time.Hour, time.Hour, 10)
	// burst not triggered — interval should be full 100ms
	if got := a.Next(); got != 100*time.Millisecond {
		t.Fatalf("inactive burst changed interval: got %v", got)
	}
}

func TestUniformArrivalIsConstant(t *testing.T) {
	a := gen.NewArrivals(gen.ArrivalUniform, 100) // 10ms interval
	want := 10 * time.Millisecond
	for range 200 {
		if got := a.Next(); got != want {
			t.Fatalf("uniform: got %v want %v", got, want)
		}
	}
}

func TestPoissonArrivalPositive(t *testing.T) {
	a := gen.NewArrivals(gen.ArrivalPoisson, 1000)
	for range 5000 {
		if d := a.Next(); d <= 0 {
			t.Fatalf("poisson: non-positive duration %v", d)
		}
	}
}

func TestPoissonArrivalMean(t *testing.T) {
	const rps = 200.0
	a := gen.NewArrivals(gen.ArrivalPoisson, rps)
	const n = 20_000
	var sum float64
	for range n {
		sum += a.Next().Seconds()
	}
	mean := sum / n
	want := 1.0 / rps
	if math.Abs(mean-want)/want > 0.10 {
		t.Fatalf("poisson mean %.6fs, want %.6fs (±10%%)", mean, want)
	}
}

// For a Poisson process the inter-arrival CV (std/mean) equals 1.
func TestPoissonArrivalCV(t *testing.T) {
	a := gen.NewArrivals(gen.ArrivalPoisson, 500)
	const n = 20_000
	samples := make([]float64, n)
	var sum float64
	for i := range n {
		samples[i] = a.Next().Seconds()
		sum += samples[i]
	}
	mean := sum / n
	var variance float64
	for _, s := range samples {
		d := s - mean
		variance += d * d
	}
	variance /= n
	cv := math.Sqrt(variance) / mean
	if math.Abs(cv-1.0) > 0.10 {
		t.Fatalf("poisson CV %.3f, want ≈1.0 (±10%%)", cv)
	}
}

func TestNewArrivalsZeroRPSClamped(t *testing.T) {
	a := gen.NewArrivals(gen.ArrivalUniform, 0)
	if d := a.Next(); d <= 0 {
		t.Fatalf("zero rps: non-positive duration %v", d)
	}
}

func TestNewArrivalsNegativeRPSClamped(t *testing.T) {
	a := gen.NewArrivals(gen.ArrivalPoisson, -5)
	if d := a.Next(); d <= 0 {
		t.Fatalf("negative rps: non-positive duration %v", d)
	}
}
