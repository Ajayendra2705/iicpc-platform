package stats

import (
	"cmp"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Recorder collects latency samples and error counts from concurrent workers.
// Percentile queries copy-then-sort the sample slice to avoid lock contention
// during computation. Replace with HDR histogram (D16) for production scale.
type Recorder struct {
	mu            sync.Mutex
	latNs         []int64
	errors        atomic.Int64
	total         atomic.Int64
	startAt       time.Time
	clockOffsetNs atomic.Int64
}

func New() *Recorder {
	return &Recorder{startAt: time.Now()}
}

func (r *Recorder) RecordLatency(ns int64) {
	r.total.Add(1)
	r.mu.Lock()
	r.latNs = append(r.latNs, ns)
	r.mu.Unlock()
}

func (r *Recorder) RecordError() {
	r.errors.Add(1)
	r.total.Add(1)
}

type Snapshot struct {
	Count         int64   `json:"count"`
	Errors        int64   `json:"errors"`
	TPS           float64 `json:"tps"`
	P50Ns         int64   `json:"p50_ns"`
	P90Ns         int64   `json:"p90_ns"`
	P99Ns         int64   `json:"p99_ns"`
	ClockOffsetNs int64   `json:"clock_offset_ns"`
}

// SetClockOffset stores the estimated clock offset (ns) relative to target.
func (r *Recorder) SetClockOffset(ns int64) {
	r.clockOffsetNs.Store(ns)
}

func (r *Recorder) Snapshot() Snapshot {
	elapsed := time.Since(r.startAt).Seconds()
	total := r.total.Load()
	tps := 0.0
	if elapsed > 0 {
		tps = float64(total) / elapsed
	}

	r.mu.Lock()
	cp := make([]int64, len(r.latNs))
	copy(cp, r.latNs)
	r.mu.Unlock()

	slices.SortFunc(cp, cmp.Compare)

	return Snapshot{
		Count:         total,
		Errors:        r.errors.Load(),
		TPS:           tps,
		P50Ns:         percentile(cp, 50),
		P90Ns:         percentile(cp, 90),
		P99Ns:         percentile(cp, 99),
		ClockOffsetNs: r.clockOffsetNs.Load(),
	}
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * float64(p) / 100.0)
	return sorted[idx]
}
