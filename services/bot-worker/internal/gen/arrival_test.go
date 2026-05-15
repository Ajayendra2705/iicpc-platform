package gen_test

import (
	"math"
	"testing"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
)

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
