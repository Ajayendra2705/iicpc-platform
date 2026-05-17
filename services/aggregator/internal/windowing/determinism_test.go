package windowing_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/rand/v2"
	"sort"
	"testing"
	"time"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

// TestReplayDeterminism is the differentiator C2 from IDEAS.md: prove that
// the scoring pipeline is *byte-identical* under replay. Given the same input
// event stream (same seed, same timestamps, same per-event payload), two
// independent runs of the aggregator must produce exactly the same Snapshots
// and the same derived score. Any drift exposes a hidden non-determinism —
// map-iteration order leaking into output, time.Now() called somewhere it
// shouldn't be, an unseeded RNG in the hot path, a goroutine race.
//
// This test runs in pure unit-test scope (no Redis, no network). It catches
// determinism regressions before they ever reach the leaderboard scoring
// path. CI-safe; runs in <1s.
func TestReplayDeterminism(t *testing.T) {
	digest1, score1 := runScoringPipeline(t)
	digest2, score2 := runScoringPipeline(t)

	if digest1 != digest2 {
		t.Errorf("snapshot stream is non-deterministic:\n  run1 sha256 = %s\n  run2 sha256 = %s", digest1, digest2)
	}
	if score1 != score2 {
		t.Errorf("derived score is non-deterministic:\n  run1 score = %v\n  run2 score = %v", score1, score2)
	}
	if t.Failed() {
		return
	}
	t.Logf("replay determinism OK: snapshot sha256 = %s, score = %+v", digest1, score1)
}

// runScoringPipeline does what the production pipeline does end-to-end for
// the purposes of this test: synthetic event generation → aggregator →
// final-snapshot scoring formula. Two invocations with identical inputs
// MUST return identical outputs.
//
// The inputs are made identical by:
//   - seeding the RNG with a fixed value
//   - using a controlled clock (windowing.WithClock) so per-window start
//     times are bit-for-bit equal
//   - sorting final snapshots by ContestantID before serialising (map
//     iteration order is the most common source of false-positive drift)
func runScoringPipeline(t *testing.T) (snapshotDigest string, score scoreVector) {
	t.Helper()

	// Fixed clock — every call returns the same instant. The aggregator only
	// reads `now()` when opening a brand-new window for a contestant; we
	// step the clock by `tick()` between simulated batches.
	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: base}

	a := windowing.New(time.Second, windowing.WithClock(clock.Now))

	// Synthetic 10K-event stream across 5 contestants. Seeded RNG → byte-
	// reproducible across runs and platforms (math/rand/v2 is portable).
	rng := rand.New(rand.NewPCG(42, 0xdeadbeef))
	contestants := []string{"alpha", "bravo", "charlie", "delta", "echo"}

	for batch := range 10 {
		// Each batch represents one 1-second window. Advance the clock
		// FIRST so the new window's start timestamp is deterministic.
		clock.Advance(time.Second)
		for range 1000 {
			c := contestants[rng.IntN(len(contestants))]
			lat := int64(rng.IntN(10_000_000) + 1) // 1ns .. 10ms
			result := pickResult(rng)
			a.Record(&telemetryv1.OrderEvent{
				ContestantId: c,
				LatencyNs:    lat,
				Result:       result,
			})
		}
		// Flush at a deterministic timestamp (NOT the wall clock).
		flushAt := base.Add(time.Duration(batch+1) * time.Second)
		a.Flush(flushAt)
	}

	// Serialize the final snapshots in a contestant-id-sorted order so map
	// iteration cannot leak into the byte stream.
	snaps := a.All()
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].ContestantID < snaps[j].ContestantID })

	out, err := json.Marshal(snaps)
	if err != nil {
		t.Fatalf("marshal snapshots: %v", err)
	}
	sum := sha256.Sum256(out)
	snapshotDigest = hex.EncodeToString(sum[:])

	// Derived score: same formula the leaderboard uses
	// (latency_norm × 0.4 + tps_norm × 0.3 + correctness × 0.3), aggregated
	// across contestants. Determinism here means: even after the score
	// rollup, no float rounding nondeterminism creeps in.
	score = computeScoreVector(snaps)
	return
}

// scoreVector is the deterministic-test stand-in for the leaderboard's
// per-contestant score row. We hash it whole, so float bit-equality is
// required — that's the strongest form of determinism we can assert.
type scoreVector struct {
	TotalTPS         float64
	WeightedP99Sum   float64
	WeightedScoreSum float64
	NumContestants   int
}

func computeScoreVector(snaps []windowing.Snapshot) scoreVector {
	sv := scoreVector{NumContestants: len(snaps)}
	for _, s := range snaps {
		latencyNorm := 1.0 / (1.0 + float64(s.P99Ns)/1_000_000.0) // arbitrary but deterministic
		tpsNorm := s.TPS / 10_000.0
		correctness := 1.0 - float64(s.Rejected+s.Timeouts)/float64(max64(s.Count, 1))
		score := latencyNorm*0.4 + tpsNorm*0.3 + correctness*0.3
		sv.TotalTPS += s.TPS
		sv.WeightedP99Sum += float64(s.P99Ns)
		sv.WeightedScoreSum += score
	}
	return sv
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// fakeClock returns a fixed instant. Advance() steps it; Now() returns the
// current instant without side effects (so multiple Now() calls within a
// single batch all return the same value — that's the whole point).
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func pickResult(r *rand.Rand) telemetryv1.OrderResult {
	switch r.IntN(100) {
	case 0, 1, 2: // 3% rejected
		return telemetryv1.OrderResult_ORDER_RESULT_REJECTED
	case 3: // 1% timeout
		return telemetryv1.OrderResult_ORDER_RESULT_TIMEOUT
	default:
		return telemetryv1.OrderResult_ORDER_RESULT_FILLED
	}
}
