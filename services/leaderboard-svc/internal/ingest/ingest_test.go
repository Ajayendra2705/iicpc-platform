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
}

func (m *mockFetcher) Snapshots(_ context.Context) ([]ingest.AggSnapshot, error) {
	return m.snaps, nil
}
func (m *mockFetcher) Reports(_ context.Context) ([]ingest.ValReport, error) {
	return m.reports, nil
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
	agg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ingest.AggSnapshot{{ContestantID: "x", P99Ns: 999}})
	}))
	defer agg.Close()
	val := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ingest.ValReport{{ContestantID: "x", Correctness: 0.9}})
	}))
	defer val.Close()

	f := ingest.NewHTTPFetcher(agg.URL, val.URL)
	snaps, err := f.Snapshots(context.Background())
	if err != nil || len(snaps) != 1 || snaps[0].ContestantID != "x" {
		t.Fatalf("Snapshots: snaps=%+v err=%v", snaps, err)
	}
	reps, err := f.Reports(context.Background())
	if err != nil || len(reps) != 1 || reps[0].Correctness != 0.9 {
		t.Fatalf("Reports: reps=%+v err=%v", reps, err)
	}
}
