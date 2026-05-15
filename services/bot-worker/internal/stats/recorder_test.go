package stats_test

import (
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/stats"
)

func TestPercentilesCorrect(t *testing.T) {
	r := stats.New()
	for i := int64(1); i <= 100; i++ {
		r.RecordLatency(i * 1000) // 1000ns … 100000ns
	}
	snap := r.Snapshot()
	// P50: idx=49, value=50000
	if snap.P50Ns != 50000 {
		t.Errorf("P50: got %d want 50000", snap.P50Ns)
	}
	// P90: idx=89, value=90000
	if snap.P90Ns != 90000 {
		t.Errorf("P90: got %d want 90000", snap.P90Ns)
	}
	// P99: idx=98, value=99000
	if snap.P99Ns != 99000 {
		t.Errorf("P99: got %d want 99000", snap.P99Ns)
	}
}

func TestErrorCount(t *testing.T) {
	r := stats.New()
	r.RecordError()
	r.RecordError()
	snap := r.Snapshot()
	if snap.Errors != 2 {
		t.Errorf("errors: got %d want 2", snap.Errors)
	}
	if snap.Count != 2 {
		t.Errorf("count: got %d want 2", snap.Count)
	}
}

func TestEmptySnapshot(t *testing.T) {
	r := stats.New()
	snap := r.Snapshot()
	if snap.P50Ns != 0 || snap.P90Ns != 0 || snap.P99Ns != 0 {
		t.Fatal("empty recorder must return zero percentiles")
	}
	if snap.Count != 0 {
		t.Fatalf("count: got %d want 0", snap.Count)
	}
}

func TestMixedLatencyAndErrors(t *testing.T) {
	r := stats.New()
	r.RecordLatency(500)
	r.RecordLatency(1500)
	r.RecordError()
	snap := r.Snapshot()
	if snap.Count != 3 {
		t.Errorf("count: got %d want 3", snap.Count)
	}
	if snap.Errors != 1 {
		t.Errorf("errors: got %d want 1", snap.Errors)
	}
}
