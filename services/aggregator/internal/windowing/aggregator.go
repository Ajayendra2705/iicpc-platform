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

// Snapshot is the rolled-up state of one window for one submission run.
type Snapshot struct {
	ContestantID string        `json:"contestant_id"`
	SubmissionID string        `json:"submission_id,omitempty"`
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

// Aggregator maintains an open window per run and rolls them on Flush. A "run"
// is keyed by submission_id when present, so each submission (attempt) is
// scored in isolation — a contestant's re-submission never blends into the
// previous attempt's histogram. Legacy events with no submission_id fall back
// to keying by contestant_id, preserving the original single-run behaviour.
type Aggregator struct {
	mu         sync.Mutex
	window     time.Duration
	historyCap int
	open       map[string]*windowState
	latest     map[string]Snapshot
	history    map[string][]*flushed // ring buffer per run key; tail is newest
	cumulative map[string]*cumState  // whole-run accumulator per run key; never evicted
	startAt    time.Time
	now        func() time.Time
}

// cumState accumulates every flushed window for one run so the score can cover
// the entire run regardless of length. The history ring above is bounded to
// historyCap windows (for the recent-window UI view); this is not, so a
// benchmark longer than historyCap seconds is still scored on all of its data.
type cumState struct {
	contestantID  string
	submissionID  string
	hist          *hdrhistogram.Histogram
	windowCount   int
	totalCount    int64
	totalRejected int64
	totalTimeouts int64
	totalDur      time.Duration
}

// merged renders the accumulated whole-run state as a MergedSnapshot.
func (c *cumState) merged() MergedSnapshot {
	avgTPS := 0.0
	if s := c.totalDur.Seconds(); s > 0 {
		avgTPS = float64(c.totalCount) / s
	}
	return MergedSnapshot{
		ContestantID:  c.contestantID,
		SubmissionID:  c.submissionID,
		WindowCount:   c.windowCount,
		Duration:      c.totalDur,
		TotalCount:    c.totalCount,
		TotalRejected: c.totalRejected,
		TotalTimeouts: c.totalTimeouts,
		AverageTPS:    avgTPS,
		P50Ns:         c.hist.ValueAtQuantile(50),
		P90Ns:         c.hist.ValueAtQuantile(90),
		P99Ns:         c.hist.ValueAtQuantile(99),
		P999Ns:        c.hist.ValueAtQuantile(99.9),
	}
}

type windowState struct {
	contestantID string
	submissionID string
	start        time.Time
	hist         *hdrhistogram.Histogram
	count        int64
	rejected     int64
	timeouts     int64
}

// runKey returns the map key for an event and the identity tags it carries.
// Keying prefers submission_id (per-attempt isolation) and falls back to
// contestant_id for legacy callers that don't set a submission.
func runKey(e *telemetryv1.OrderEvent) (key, contestant, submission string) {
	contestant = e.GetContestantId()
	submission = e.GetSubmissionId()
	if submission != "" {
		return submission, contestant, submission
	}
	return contestant, contestant, submission
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
		cumulative: make(map[string]*cumState),
		startAt:    time.Now(),
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Record adds an OrderEvent to the open window for its run (submission, or
// contestant when no submission is set). Safe to call from any goroutine.
func (a *Aggregator) Record(e *telemetryv1.OrderEvent) {
	if e == nil || e.GetContestantId() == "" {
		return
	}
	key, contestant, submission := runKey(e)
	a.mu.Lock()
	defer a.mu.Unlock()

	ws, ok := a.open[key]
	if !ok {
		ws = &windowState{
			contestantID: contestant,
			submissionID: submission,
			start:        a.now(),
			hist:         hdrhistogram.New(defaultMinLatNs, defaultMaxLatNs, defaultSigDigits),
		}
		a.open[key] = ws
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
	for key, ws := range a.open {
		dur := now.Sub(ws.start)
		if dur <= 0 {
			dur = a.window
		}
		tps := 0.0
		if secs := dur.Seconds(); secs > 0 {
			tps = float64(ws.count) / secs
		}
		snap := Snapshot{
			ContestantID: ws.contestantID,
			SubmissionID: ws.submissionID,
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
		a.latest[key] = snap
		// Keep the histogram alongside the snapshot in the ring so MergedRecent
		// can recompute exact percentiles over a multi-window range.
		ring := a.history[key]
		ring = append(ring, &flushed{snap: snap, hist: ws.hist})
		if len(ring) > a.historyCap {
			ring = ring[len(ring)-a.historyCap:]
		}
		a.history[key] = ring

		// Fold this window into the whole-run accumulator. Unlike the ring, this
		// is never evicted, so a run longer than historyCap windows is still
		// scored on all of its data (windows<=0 in MergedRecent reads this).
		cs, ok := a.cumulative[key]
		if !ok {
			cs = &cumState{
				contestantID: ws.contestantID,
				submissionID: ws.submissionID,
				hist:         hdrhistogram.New(defaultMinLatNs, defaultMaxLatNs, defaultSigDigits),
			}
			a.cumulative[key] = cs
		}
		cs.hist.Merge(ws.hist)
		cs.windowCount++
		cs.totalCount += ws.count
		cs.totalRejected += ws.rejected
		cs.totalTimeouts += ws.timeouts
		cs.totalDur += dur
	}
	// reset open windows
	a.open = make(map[string]*windowState)
	return out
}

// latestRunKeyForContestant returns the run key of the contestant's most
// recently flushed submission, scanning the latest snapshots. It lets the
// per-contestant detail endpoints resolve a contestant_id even though runs are
// keyed by submission_id once attempts carry one. Caller must hold a.mu.
func (a *Aggregator) latestRunKeyForContestant(contestantID string) (string, bool) {
	var (
		bestKey   string
		bestStart time.Time
		found     bool
	)
	for key, snap := range a.latest {
		if snap.ContestantID != contestantID {
			continue
		}
		if !found || snap.WindowStart.After(bestStart) {
			bestKey, bestStart, found = key, snap.WindowStart, true
		}
	}
	return bestKey, found
}

// MergedRecent returns a percentile cut for the run keyed by key (a
// submission_id, or a contestant_id for legacy runs). Histograms are merged
// bucket-by-bucket so the result is exact — NOT an average of per-window
// percentiles (which would be wrong).
//
// windows<=0 merges the entire run via the never-evicted cumulative accumulator,
// so a benchmark longer than historyCap seconds is still scored on all of its
// data (this is the scoring path). windows>0 merges the last N windows from the
// bounded ring (the recent-window UI view), clamped to what's retained.
//
// When key matches no run directly it is treated as a contestant_id and resolved
// to that contestant's most recent submission, so /metrics/merged/{contestant_id}
// still works once attempts carry a submission_id. Returns false if nothing has
// been flushed for the key or any of the contestant's submissions.
func (a *Aggregator) MergedRecent(key string, windows int) (MergedSnapshot, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	effKey := key
	if _, ok := a.history[effKey]; !ok {
		// key may be a contestant_id while runs are keyed by submission_id.
		rk, found := a.latestRunKeyForContestant(key)
		if !found {
			return MergedSnapshot{}, false
		}
		effKey = rk
	}

	// Whole-run scoring path: read the cumulative accumulator covering every
	// flushed window, not just the last historyCap of them.
	if windows <= 0 {
		cs, ok := a.cumulative[effKey]
		if !ok || cs.windowCount == 0 {
			return MergedSnapshot{}, false
		}
		return cs.merged(), true
	}

	ring := a.history[effKey]
	if len(ring) == 0 {
		return MergedSnapshot{}, false
	}
	if windows > len(ring) {
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
	// Every window in a ring shares the same run key, hence the same identity
	// tags; read them off the newest flushed snapshot.
	newest := slice[len(slice)-1].snap
	return MergedSnapshot{
		ContestantID:  newest.ContestantID,
		SubmissionID:  newest.SubmissionID,
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

// AllMerged returns the merged snapshot for every run that has at least one
// flushed window. windows<=0 merges the run's entire history via the cumulative
// accumulator (no historyCap limit); a positive value bounds it to the last N
// retained windows. Percentiles are exact bucket-by-bucket merges, never
// averages of per-window percentiles — this is the snapshot the leaderboard
// scores on (windows=0), so a contestant is ranked by its true whole-run tail
// latency, not one jittery window and not just the last historyCap seconds.
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

// Latest returns the most recently flushed snapshot for the given id. The id is
// first tried as a run key (a submission_id, or a contestant_id for legacy
// runs); if that misses it is resolved as a contestant_id to that contestant's
// most recent submission, so /metrics/{contestant_id} still works once attempts
// carry a submission_id.
func (a *Aggregator) Latest(id string) (Snapshot, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s, ok := a.latest[id]; ok {
		return s, true
	}
	if key, ok := a.latestRunKeyForContestant(id); ok {
		return a.latest[key], true
	}
	return Snapshot{}, false
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
