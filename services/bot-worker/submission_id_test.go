package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/client"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/stats"
)

// recordingTelemetry captures every emitted OrderEvent for assertions.
type recordingTelemetry struct {
	mu     sync.Mutex
	events []*telemetryv1.OrderEvent
}

func (r *recordingTelemetry) Emit(e *telemetryv1.OrderEvent) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}
func (r *recordingTelemetry) Close() error { return nil }
func (r *recordingTelemetry) snapshot() []*telemetryv1.OrderEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*telemetryv1.OrderEvent(nil), r.events...)
}

// TestRunWorkerStampsSubmissionID proves every emitted telemetry event carries
// the contestant_id AND submission_id tags so the aggregator can score each
// submission in isolation and the leaderboard can rank by best attempt.
func TestRunWorkerStampsSubmissionID(t *testing.T) {
	var orderSeq atomic.Uint64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/order"):
			id := orderSeq.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("ord-%d", id), "fills": []any{}})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rec := stats.New()
	tele := &recordingTelemetry{}
	arrivals := gen.NewArrivals(gen.ArrivalPoisson, 200.0)
	cli := client.New(client.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	g := gen.New(gen.Config{MidPrice: 100, PriceSigma: 1, CancelRatio: 0.3})
	runWorker(ctx, 0, cli, g, rec, arrivals, tele, "team-alpha", "sub-42", "bot-0", log)

	events := tele.snapshot()
	if len(events) == 0 {
		t.Fatal("no telemetry events emitted")
	}
	for i, e := range events {
		if e.GetContestantId() != "team-alpha" {
			t.Fatalf("event %d contestant_id=%q want team-alpha", i, e.GetContestantId())
		}
		if e.GetSubmissionId() != "sub-42" {
			t.Fatalf("event %d submission_id=%q want sub-42", i, e.GetSubmissionId())
		}
	}
}
