package ingest_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ingest"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/score"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/store"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ws"
)

type mockFetcher struct {
	snaps   []ingest.AggSnapshot
	reports []ingest.ValReport
	crashes []ingest.CrashReport
}

func (m *mockFetcher) Snapshots(_ context.Context) ([]ingest.AggSnapshot, error) {
	return m.snaps, nil
}
func (m *mockFetcher) Reports(_ context.Context) ([]ingest.ValReport, error) {
	return m.reports, nil
}
func (m *mockFetcher) Crashes(_ context.Context) ([]ingest.CrashReport, error) {
	return m.crashes, nil
}

func newIngester(f ingest.Fetcher) (*ingest.Ingester, *store.Stub, *ws.Hub) {
	s := store.NewStub()
	h := ws.NewHub()
	return &ingest.Ingester{
		Fetcher:  f,
		Calc:     score.New(score.DefaultConfig()),
		Store:    s,
		Hub:      h,
		TopN:     10,
		Interval: 100 * time.Millisecond,
		Log:      slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})),
	}, s, h
}

func TestTickUpsertsScores(t *testing.T) {
	f := &mockFetcher{
		snaps: []ingest.AggSnapshot{
			{ContestantID: "alpha", P99Ns: 1_000, TPS: 50_000},
			{ContestantID: "beta", P99Ns: 500_000_000, TPS: 0},
		},
		reports: []ingest.ValReport{
			{ContestantID: "alpha", Correctness: 1.0},
			{ContestantID: "beta", Correctness: 0.5},
		},
	}
	ing, s, _ := newIngester(f)
	if err := ing.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	top, _ := s.Top(context.Background(), 0)
	if len(top) != 2 {
		t.Fatalf("Top: got %d want 2", len(top))
	}
	if top[0].ContestantID != "alpha" {
		t.Errorf("rank 1: got %s want alpha", top[0].ContestantID)
	}
	if top[0].Score <= top[1].Score {
		t.Errorf("scores: %d should beat %d", top[0].Score, top[1].Score)
	}
}

// TestTickRanksContestantByBestSubmission walks a contestant through several
// submissions (as the aggregator would surface them at /metrics/merged) and
// asserts the leaderboard settles on the best attempt, not the latest. A
// high-TPS/low-latency middle attempt must keep the contestant ranked even
// after a poor final attempt — the end-to-end proof of best-submission ranking.
func TestTickRanksContestantByBestSubmission(t *testing.T) {
	// Three attempts by "carol": good, GREAT, then bad. Plus a rival "dave".
	f := &mockFetcher{
		snaps: []ingest.AggSnapshot{
			{ContestantID: "carol", SubmissionID: "c-good", P99Ns: 2_000_000, TPS: 8_000},
			{ContestantID: "carol", SubmissionID: "c-great", P99Ns: 1_000, TPS: 50_000},
			{ContestantID: "carol", SubmissionID: "c-bad", P99Ns: 500_000_000, TPS: 100},
			{ContestantID: "dave", SubmissionID: "d-1", P99Ns: 1_500_000, TPS: 9_000},
		},
		reports: []ingest.ValReport{
			{ContestantID: "carol", Correctness: 1.0},
			{ContestantID: "dave", Correctness: 1.0},
		},
	}
	ing, s, _ := newIngester(f)
	if err := ing.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	top, _ := s.Top(context.Background(), 0)
	if len(top) != 2 {
		t.Fatalf("Top: got %d want 2 (one row per contestant, best-of)", len(top))
	}
	// carol must be ranked on c-great (the bad final attempt must not drag her
	// down), and that beats dave's single solid run.
	if top[0].ContestantID != "carol" {
		t.Errorf("rank 1: got %s want carol (ranked on her best submission)", top[0].ContestantID)
	}
	// Independently score c-great alone to confirm carol's leaderboard score
	// equals her best attempt's score, not the latest/worst.
	great := score.New(score.DefaultConfig()).Compute(score.Inputs{P99Ns: 1_000, TPS: 50_000, Correctness: 1.0})
	if top[0].Score != great.FinalScore {
		t.Errorf("carol score=%d, want %d (her best submission c-great)", top[0].Score, great.FinalScore)
	}
}

// TestTickAttributesCorrectnessAndCrashesPerSubmission proves a contestant's
// two submissions are scored with their OWN correctness and crash counts, not a
// blended contestant-wide value. s-clean (perfect, no crash) must outscore
// s-buggy (low correctness + a crash) for the SAME contestant, and best-of then
// ranks the contestant on s-clean.
func TestTickAttributesCorrectnessAndCrashesPerSubmission(t *testing.T) {
	f := &mockFetcher{
		snaps: []ingest.AggSnapshot{
			{ContestantID: "carol", SubmissionID: "s-clean", P99Ns: 1_000, TPS: 50_000},
			{ContestantID: "carol", SubmissionID: "s-buggy", P99Ns: 1_000, TPS: 50_000},
		},
		reports: []ingest.ValReport{
			{ContestantID: "carol", SubmissionID: "s-clean", Correctness: 1.0, TotalChecked: 100},
			{ContestantID: "carol", SubmissionID: "s-buggy", Correctness: 0.2, TotalChecked: 100, Mismatches: 80},
		},
		crashes: []ingest.CrashReport{
			{ContestantID: "carol", SubmissionID: "s-buggy", Crashes: 1},
		},
	}
	ing, s, _ := newIngester(f)
	if err := ing.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	top, _ := s.Top(context.Background(), 0)
	if len(top) != 1 {
		t.Fatalf("Top: got %d want 1 (best-of for carol)", len(top))
	}
	// carol's leaderboard score must equal the clean submission's score —
	// proving the buggy attempt's low correctness + crash were applied only to
	// s-buggy, and best-of kept s-clean.
	clean := score.New(score.DefaultConfig()).Compute(score.Inputs{P99Ns: 1_000, TPS: 50_000, Correctness: 1.0})
	if top[0].Score != clean.FinalScore {
		t.Errorf("carol score=%d want %d (clean submission, per-submission attribution)", top[0].Score, clean.FinalScore)
	}
	// Sanity: the buggy submission with a crash floors to 0 (10k penalty), so if
	// attribution had leaked contestant-wide, carol's best would NOT be clean.
	if clean.FinalScore == 0 {
		t.Fatal("test precondition: clean submission should score > 0")
	}
}

func TestTickBroadcastsTopN(t *testing.T) {
	f := &mockFetcher{
		snaps:   []ingest.AggSnapshot{{ContestantID: "a", P99Ns: 1_000, TPS: 50_000}},
		reports: []ingest.ValReport{{ContestantID: "a", Correctness: 1.0}},
	}
	ing, _, h := newIngester(f)
	c := h.Register()

	if err := ing.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	select {
	case msg := <-c.Send():
		if !strings.Contains(string(msg), "\"top\"") {
			t.Errorf("payload missing top: %s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast received")
	}
}

func TestTickMissingValidatorAssumesCorrect(t *testing.T) {
	f := &mockFetcher{
		snaps:   []ingest.AggSnapshot{{ContestantID: "a", P99Ns: 1_000, TPS: 50_000}},
		reports: nil,
	}
	ing, s, _ := newIngester(f)
	if err := ing.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	top, _ := s.Top(context.Background(), 0)
	if len(top) != 1 || top[0].Score < 900 {
		t.Errorf("expected ~perfect score with no validator: %+v", top)
	}
}

func TestHTTPFetcherSnapshots(t *testing.T) {
	// Aggregator now serves the whole-run merged snapshot at /metrics/merged;
	// the fetcher must hit that path and map avg_tps → TPS.
	agg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics/merged" {
			http.Error(w, "wrong path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"contestant_id":"x","count":42,"timeouts":1,"avg_tps":1234.5,"p99_ns":999}]`)
	}))
	defer agg.Close()
	val := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ingest.ValReport{{ContestantID: "x", Correctness: 0.9}})
	}))
	defer val.Close()

	sbx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ingest.CrashReport{{ContestantID: "x", Crashes: 2}})
	}))
	defer sbx.Close()

	f := ingest.NewHTTPFetcher(agg.URL, val.URL, sbx.URL)
	snaps, err := f.Snapshots(context.Background())
	if err != nil || len(snaps) != 1 || snaps[0].ContestantID != "x" {
		t.Fatalf("Snapshots: snaps=%+v err=%v", snaps, err)
	}
	if snaps[0].TPS != 1234.5 || snaps[0].P99Ns != 999 || snaps[0].Timeouts != 1 {
		t.Fatalf("merged fields not mapped: %+v", snaps[0])
	}
	reps, err := f.Reports(context.Background())
	if err != nil || len(reps) != 1 || reps[0].Correctness != 0.9 {
		t.Fatalf("Reports: reps=%+v err=%v", reps, err)
	}
	crs, err := f.Crashes(context.Background())
	if err != nil || len(crs) != 1 || crs[0].Crashes != 2 {
		t.Fatalf("Crashes: crs=%+v err=%v", crs, err)
	}
}

func TestHTTPFetcherCrashesDisabledWhenURLEmpty(t *testing.T) {
	f := ingest.NewHTTPFetcher("http://agg", "http://val", "")
	crs, err := f.Crashes(context.Background())
	if err != nil || crs != nil {
		t.Fatalf("expected no crashes when URL empty: crs=%+v err=%v", crs, err)
	}
}

func TestTickAppliesCrashPenalty(t *testing.T) {
	// Same perfect metrics for both; only "doomed" has a crash, which the
	// 10k penalty floors to 0 — proving the crash signal reaches the score.
	f := &mockFetcher{
		snaps: []ingest.AggSnapshot{
			{ContestantID: "healthy", P99Ns: 1_000, TPS: 50_000},
			{ContestantID: "doomed", P99Ns: 1_000, TPS: 50_000},
		},
		reports: []ingest.ValReport{
			{ContestantID: "healthy", Correctness: 1.0},
			{ContestantID: "doomed", Correctness: 1.0},
		},
		crashes: []ingest.CrashReport{{ContestantID: "doomed", Crashes: 1}},
	}
	ing, s, _ := newIngester(f)
	if err := ing.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	top, _ := s.Top(context.Background(), 0)
	scores := map[string]int64{}
	for _, e := range top {
		scores[e.ContestantID] = e.Score
	}
	if scores["healthy"] <= 0 {
		t.Errorf("healthy contestant should score > 0, got %d", scores["healthy"])
	}
	if scores["doomed"] != 0 {
		t.Errorf("crashed contestant should be floored to 0, got %d", scores["doomed"])
	}
}
