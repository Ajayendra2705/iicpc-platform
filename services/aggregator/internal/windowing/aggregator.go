// Package windowing computes 1-second tumbling-window percentiles per
// contestant using HDR histograms. Latencies are in nanoseconds.
package windowing

import (
	"sync"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Defaults give 0.1% precision from 1ns up to 60s — wide enough to capture
// loopback (sub-µs) and timeout outliers (tens of seconds).
const (
	defaultMinLatNs   int64 = 1
	defaultMaxLatNs   int64 = 60_000_000_000
	defaultSigDigits  int   = 3
	defaultWindowSize       = time.Second
)

// Snapshot is the rolled-up state of one window for one contestant.
type Snapshot struct {
	ContestantID string        `json:"contestant_id"`
	WindowStart  time.Time     `json:"window_start"`
	Duration     time.Duration `json:"duration_ns"`
	Count        int64         `json:"count"`
	Rejected     int64         `json:"rejected"`
	Timeouts     int64         `json:"timeouts"`
	TPS          float64       `json:"tps"`
	P50Ns        int64         `json:"p50_ns"`
	P90Ns        int64         `json:"p90_ns"`
	P99Ns        int64         `json:"p99_ns"`
	P999Ns       int64         `json:"p999_ns"`
}

// DefaultHistoryWindows is the size of the per-contestant ring of recently
// flushed windows. 60 × 1s = a 1-minute mergeable history matching the
// rolling-window TPS chart in the UI.
const DefaultHistoryWindows = 60

// Aggregator maintains an open window per contestant and rolls them on Flush.
type Aggregator struct {
	mu         sync.Mutex
	window     time.Duration
	historyCap int
	open       map[string]*windowState
	latest     map[string]Snapshot
	history    map[string][]*flushed // ring buffer per contestant; tail is newest
	startAt    time.Time
	now        func() time.Time
}

type windowState struct {
	start    time.Time
	hist     *hdrhistogram.Histogram
	count    int64
	rejected int64
	timeouts int64
}

// flushed pairs a Snapshot with the histogram so MergedRange can recompute
// percentiles exactly (averaging percentiles across windows is wrong).
type flushed struct {
	snap Snapshot
	hist *hdrhistogram.Histogram
}

// Option configures a new Aggregator. Use with New(window, opts...).
type Option func(*Aggregator)

// WithClock injects a clock function used for every time observation inside
// the aggregator (open-window start times and the recorded startAt). Required
// for replay-determinism tests where two runs must observe identical
// timestamps; production code leaves it unset to use the real wall clock.
func WithClock(now func() time.Time) Option {
	return func(a *Aggregator) {
		if now != nil {
			a.now = now
			a.startAt = now()
		}
	}
}

func New(window time.Duration, opts ...Option) *Aggregator {
	if window <= 0 {
		window = defaultWindowSize
	}
	a := &Aggregator{
		window:     window,
		historyCap: DefaultHistoryWindows,
		open:       make(map[string]*windowState),
		latest:     make(map[string]Snapshot),
		history:    make(map[string][]*flushed),
		startAt:    time.Now(),
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Record adds an OrderEvent to the open window for its contestant. Safe to
// call from any goroutine.
func (a *Aggregator) Record(e *telemetryv1.OrderEvent) {
	if e == nil || e.GetContestantId() == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	ws, ok := a.open[e.GetContestantId()]
	if !ok {
		ws = &windowState{
			start: a.now(),
			hist:  hdrhistogram.New(defaultMinLatNs, defaultMaxLatNs, defaultSigDigits),
		}
		a.open[e.GetContestantId()] = ws
	}

	if lat := e.GetLatencyNs(); lat > 0 {
		_ = ws.hist.RecordValue(lat)
	}
	ws.count++
	switch e.GetResult() {
	case telemetryv1.OrderResult_ORDER_RESULT_REJECTED:
		ws.rejected++
	case telemetryv1.OrderResult_ORDER_RESULT_TIMEOUT:
		ws.timeouts++
	}
}

// Flush rolls every open window into a Snapshot, stores it as the latest
// snapshot for that contestant, and resets the window. Returns the slice of
// snapshots produced (one per active contestant).
func (a *Aggregator) Flush(now time.Time) []Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.open) == 0 {
		return nil
	}
	out := make([]Snapshot, 0, len(a.open))
	for id, ws := range a.open {
		dur := now.Sub(ws.start)
		if dur <= 0 {
			dur = a.window
		}
		tps := 0.0
		if secs := dur.Seconds(); secs > 0 {
			tps = float64(ws.count) / secs
		}
		snap := Snapshot{
			ContestantID: id,
			WindowStart:  ws.start,
			Duration:     dur,
			Count:        ws.count,
			Rejected:     ws.rejected,
			Timeouts:     ws.timeouts,
			TPS:          tps,
			P50Ns:        ws.hist.ValueAtQuantile(50),
			P90Ns:        ws.hist.ValueAtQuantile(90),
			P99Ns:        ws.hist.ValueAtQuantile(99),
			P999Ns:       ws.hist.ValueAtQuantile(99.9),
		}
		out = append(out, snap)
		a.latest[id] = snap
		// Keep the histogram alongside the snapshot in the ring so MergedRecent
		// can recompute exact percentiles over a multi-window range.
		ring := a.history[id]
		ring = append(ring, &flushed{snap: snap, hist: ws.hist})
		if len(ring) > a.historyCap {
			ring = ring[len(ring)-a.historyCap:]
		}
		a.history[id] = ring
	}
	// reset open windows
	a.open = make(map[string]*windowState)
	return out
}

// MergedRecent returns a percentile cut over the last N flushed windows for
// the contestant. Histograms are merged bucket-by-bucket so the result is
// exact — NOT an average of per-window percentiles (which would be wrong).
// If fewer than N windows are available, merges whatever's there. Returns
// false if no windows have been flushed yet for this contestant.
func (a *Aggregator) MergedRecent(contestantID string, windows int) (MergedSnapshot, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	ring, ok := a.history[contestantID]
	if !ok || len(ring) == 0 {
		return MergedSnapshot{}, false
	}
	if windows <= 0 || windows > len(ring) {
		windows = len(ring)
	}
	slice := ring[len(ring)-windows:]

	hists := make([]*hdrhistogram.Histogram, 0, len(slice))
	var totalCount, totalRejected, totalTimeouts int64
	var totalDur time.Duration
	for _, f := range slice {
		hists = append(hists, f.hist)
		totalCount += f.snap.Count
		totalRejected += f.snap.Rejected
		totalTimeouts += f.snap.Timeouts
		totalDur += f.snap.Duration
	}
	merged := MergeHistograms(hists)
	if merged == nil {
		return MergedSnapshot{}, false
	}
	avgTPS := 0.0
	if s := totalDur.Seconds(); s > 0 {
		avgTPS = float64(totalCount) / s
	}
	return MergedSnapshot{
		ContestantID:  contestantID,
		WindowCount:   len(slice),
		Duration:      totalDur,
		TotalCount:    totalCount,
		TotalRejected: totalRejected,
		TotalTimeouts: totalTimeouts,
		AverageTPS:    avgTPS,
		P50Ns:         merged.ValueAtQuantile(50),
		P90Ns:         merged.ValueAtQuantile(90),
		P99Ns:         merged.ValueAtQuantile(99),
		P999Ns:        merged.ValueAtQuantile(99.9),
	}, true
}

// AllMerged returns the whole-history merged snapshot for every contestant
// that has at least one flushed window. windows<=0 merges the entire retained
// history (up to historyCap); a positive value bounds it to the last N windows.
// Percentiles are exact bucket-by-bucket merges, never averages of per-window
// percentiles — this is the snapshot the leaderboard scores on, so a
// contestant is ranked by its whole-run tail latency, not one jittery window.
func (a *Aggregator) AllMerged(windows int) []MergedSnapshot {
	a.mu.Lock()
	ids := make([]string, 0, len(a.history))
	for id := range a.history {
		ids = append(ids, id)
	}
	a.mu.Unlock()

	out := make([]MergedSnapshot, 0, len(ids))
	for _, id := range ids {
		// MergedRecent takes the lock itself; call it outside our critical section.
		if m, ok := a.MergedRecent(id, windows); ok {
			out = append(out, m)
		}
	}
	return out
}

// Latest returns the most recently flushed snapshot for the contestant.
func (a *Aggregator) Latest(contestantID string) (Snapshot, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.latest[contestantID]
	return s, ok
}

// All returns a copy of every contestant's latest snapshot.
func (a *Aggregator) All() []Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Snapshot, 0, len(a.latest))
	for _, s := range a.latest {
		out = append(out, s)
	}
	return out
}
