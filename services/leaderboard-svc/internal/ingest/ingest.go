// Package ingest pulls per-contestant data from the aggregator and validator,
// computes a composite score, and upserts to the leaderboard store + WS hub.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/score"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/store"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ws"
)

// Fetcher pulls upstream metrics. HTTPFetcher is the prod implementation;
// tests inject a mock.
type Fetcher interface {
	Snapshots(ctx context.Context) ([]AggSnapshot, error)
	Reports(ctx context.Context) ([]ValReport, error)
}

// Ingester ticks every Interval, fetches metrics, computes scores, writes
// them to the store, and broadcasts the new top-N over the WS hub.
type Ingester struct {
	Fetcher  Fetcher
	Calc     *score.Calculator
	Store    store.Store
	Hub      *ws.Hub
	TopN     int
	Interval time.Duration
	Log      *slog.Logger
}

// Run blocks until ctx is done, executing Tick every Interval.
func (i *Ingester) Run(ctx context.Context) {
	tick := time.NewTicker(i.Interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := i.Tick(ctx); err != nil {
				i.Log.Warn("ingest tick", "err", err)
			}
		}
	}
}

// Tick performs a single fetch → score → upsert → broadcast cycle.
func (i *Ingester) Tick(ctx context.Context) error {
	snaps, err := i.Fetcher.Snapshots(ctx)
	if err != nil {
		return fmt.Errorf("fetch snapshots: %w", err)
	}
	reports, err := i.Fetcher.Reports(ctx)
	if err != nil {
		return fmt.Errorf("fetch reports: %w", err)
	}

	correctness := make(map[string]float64, len(reports))
	for _, r := range reports {
		correctness[r.ContestantID] = r.Correctness
	}

	for _, s := range snaps {
		corr, ok := correctness[s.ContestantID]
		if !ok {
			corr = 1.0 // no validator data yet — assume correct
		}
		r := i.Calc.Compute(score.Inputs{
			P99Ns:       s.P99Ns,
			TPS:         s.TPS,
			Correctness: corr,
			Timeouts:    s.Timeouts,
		})
		if err := i.Store.Upsert(ctx, s.ContestantID, r.FinalScore); err != nil {
			i.Log.Warn("store upsert", "id", s.ContestantID, "err", err)
			continue
		}
	}

	top, err := i.Store.Top(ctx, i.TopN)
	if err != nil {
		return fmt.Errorf("store top: %w", err)
	}
	payload, err := json.Marshal(map[string]any{"top": top, "at_unix_ms": time.Now().UnixMilli()})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	i.Hub.Broadcast(payload)
	return nil
}

// HTTPFetcher hits aggregator and validator HTTP endpoints.
type HTTPFetcher struct {
	AggregatorURL string // e.g. http://aggregator:8084
	ValidatorURL  string // e.g. http://validator:8085
	HC            *http.Client
}

func NewHTTPFetcher(aggURL, valURL string) *HTTPFetcher {
	return &HTTPFetcher{
		AggregatorURL: aggURL,
		ValidatorURL:  valURL,
		HC:            &http.Client{Timeout: 3 * time.Second},
	}
}

func (f *HTTPFetcher) Snapshots(ctx context.Context) ([]AggSnapshot, error) {
	var out []AggSnapshot
	if err := f.getJSON(ctx, f.AggregatorURL+"/metrics", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (f *HTTPFetcher) Reports(ctx context.Context) ([]ValReport, error) {
	var out []ValReport
	if err := f.getJSON(ctx, f.ValidatorURL+"/validate", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (f *HTTPFetcher) getJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := f.HC.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
