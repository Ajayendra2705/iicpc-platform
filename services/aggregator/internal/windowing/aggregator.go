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

// Aggregator maintains an open window per contestant and rolls them on Flush.
type Aggregator struct {
	mu      sync.Mutex
	window  time.Duration
	open    map[string]*windowState
	latest  map[string]Snapshot
	startAt time.Time
	now     func() time.Time
}

type windowState struct {
	start    time.Time
	hist     *hdrhistogram.Histogram
	count    int64
	rejected int64
	timeouts int64
}

func New(window time.Duration) *Aggregator {
	if window <= 0 {
		window = defaultWindowSize
	}
	return &Aggregator{
		window:  window,
		open:    make(map[string]*windowState),
		latest:  make(map[string]Snapshot),
		startAt: time.Now(),
		now:     time.Now,
	}
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
	}
	// reset open windows
	a.open = make(map[string]*windowState)
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
