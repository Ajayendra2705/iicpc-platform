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
	// Crashes returns per-contestant crash counts. Optional: implementations
	// return an empty slice when no crash source is configured.
	Crashes(ctx context.Context) ([]CrashReport, error)
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

	// Correctness is matched to a submission when the validator scored per
	// submission, falling back to the contestant-level value for legacy reports.
	//
	// A correctness value is only meaningful when the validator actually checked
	// at least one authoritative fill. A report with TotalChecked==0 (e.g. a
	// submission whose every event was UNSPECIFIED or cancel-only) carries the
	// validator's default correctness=1.0 — crediting it would hand full marks to
	// an engine we never verified. Treat such reports as absent so the fail-closed
	// skip below applies until real checks land.
	corrBySubmission := make(map[string]float64)
	corrByContestant := make(map[string]float64, len(reports))
	for _, r := range reports {
		if r.TotalChecked <= 0 {
			continue
		}
		if r.SubmissionID != "" {
			corrBySubmission[r.SubmissionID] = r.Correctness
		}
		corrByContestant[r.ContestantID] = r.Correctness
	}

	// Crashes are optional — if the sandbox-runner crash source is unreachable
	// or not configured, score with zero crashes rather than failing the tick.
	// Keyed per submission with a contestant-level fallback, same as correctness.
	crashBySubmission := make(map[string]int64)
	crashByContestant := make(map[string]int64)
	if crashReports, err := i.Fetcher.Crashes(ctx); err != nil {
		i.Log.Warn("fetch crashes", "err", err)
	} else {
		for _, c := range crashReports {
			if c.SubmissionID != "" {
				crashBySubmission[c.SubmissionID] = c.Crashes
			}
			crashByContestant[c.ContestantID] += c.Crashes
		}
	}

	// Each snapshot is one submission's whole-run merged metrics. We score every
	// submission with its OWN correctness and crash counts and let the store keep
	// the contestant's best across attempts.
	//
	// Attribution is strict AND fail-closed: a submission is scored only once the
	// validator has reported a verdict for it. A snapshot tagged with a
	// submission_id is scored solely from that submission's validator/crash rows —
	// never the contestant aggregate, or a clean attempt would inherit a sibling
	// attempt's crash (an attempt with no crash row genuinely had zero crashes).
	// If no validator verdict exists yet we SKIP the submission rather than assume
	// correctness: scoring it fail-open would hand a fast-but-wrong engine free
	// correctness credit until the validator caught up. The contestant-level maps
	// are used only for legacy snapshots that carry no submission_id at all.
	for _, s := range snaps {
		var corr float64
		var crashes int64
		if s.SubmissionID != "" {
			c, ok := corrBySubmission[s.SubmissionID]
			if !ok {
				continue // no validator verdict for this attempt yet — don't score it
			}
			corr = c
			crashes = crashBySubmission[s.SubmissionID] // absent => 0 (no crash)
		} else {
			c, ok := corrByContestant[s.ContestantID]
			if !ok {
				continue // legacy snapshot with no validator verdict — skip
			}
			corr = c
			crashes = crashByContestant[s.ContestantID]
		}
		r := i.Calc.Compute(score.Inputs{
			P99Ns:       s.P99Ns,
			TPS:         s.TPS,
			Correctness: corr,
			Crashes:     crashes,
			Timeouts:    s.Timeouts,
		})
		if err := i.Store.UpsertSubmission(ctx, s.ContestantID, s.SubmissionID, r.FinalScore); err != nil {
			i.Log.Warn("store upsert submission", "contestant", s.ContestantID, "submission", s.SubmissionID, "err", err)
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

// HTTPFetcher hits aggregator, validator, and sandbox-runner HTTP endpoints.
type HTTPFetcher struct {
	AggregatorURL    string // e.g. http://aggregator:8084
	ValidatorURL     string // e.g. http://validator:8085
	SandboxRunnerURL string // e.g. http://sandbox-runner:8090 ("" disables crashes)
	HC               *http.Client
}

func NewHTTPFetcher(aggURL, valURL, sandboxURL string) *HTTPFetcher {
	return &HTTPFetcher{
		AggregatorURL:    aggURL,
		ValidatorURL:     valURL,
		SandboxRunnerURL: sandboxURL,
		HC:               &http.Client{Timeout: 3 * time.Second},
	}
}

// mergedRow mirrors the aggregator's MergedSnapshot JSON (GET /metrics/merged).
// We score on the whole-run merged histogram — exact bucket-merged percentiles
// and cumulative TPS — rather than the last 1-second window, so a contestant's
// rank reflects its entire run, not one jittery window.
type mergedRow struct {
	ContestantID string  `json:"contestant_id"`
	SubmissionID string  `json:"submission_id"`
	Count        int64   `json:"count"`
	Rejected     int64   `json:"rejected"`
	Timeouts     int64   `json:"timeouts"`
	AvgTPS       float64 `json:"avg_tps"`
	P50Ns        int64   `json:"p50_ns"`
	P90Ns        int64   `json:"p90_ns"`
	P99Ns        int64   `json:"p99_ns"`
}

func (f *HTTPFetcher) Snapshots(ctx context.Context) ([]AggSnapshot, error) {
	var rows []mergedRow
	if err := f.getJSON(ctx, f.AggregatorURL+"/metrics/merged", &rows); err != nil {
		return nil, err
	}
	out := make([]AggSnapshot, len(rows))
	for i, r := range rows {
		out[i] = AggSnapshot{
			ContestantID: r.ContestantID,
			SubmissionID: r.SubmissionID,
			Count:        r.Count,
			Rejected:     r.Rejected,
			Timeouts:     r.Timeouts,
			TPS:          r.AvgTPS,
			P50Ns:        r.P50Ns,
			P90Ns:        r.P90Ns,
			P99Ns:        r.P99Ns,
		}
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

func (f *HTTPFetcher) Crashes(ctx context.Context) ([]CrashReport, error) {
	if f.SandboxRunnerURL == "" {
		return nil, nil // crash source not configured
	}
	var out []CrashReport
	if err := f.getJSON(ctx, f.SandboxRunnerURL+"/crashes", &out); err != nil {
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
