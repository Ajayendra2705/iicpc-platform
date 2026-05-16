// Package seeder injects synthetic contestants with drifting scores. Used to
// drive the leaderboard UI without spinning up the full bot → telemetry →
// aggregator → validator pipeline. Enable with SEED_DEMO=true.
package seeder

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/store"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ws"
)

// Seed contestants and starting scores. Order doesn't matter — they drift.
var defaultContestants = []struct {
	ID    string
	Score int64
}{
	{"team-alpha", 880},
	{"team-bravo", 740},
	{"team-charlie", 690},
	{"team-delta", 610},
	{"team-echo", 540},
	{"team-foxtrot", 420},
}

const (
	maxScore   int64 = 1000
	driftRange int   = 51 // [0,51) → ±25 per tick
)

// Run upserts each demo contestant's drifting score on every tick and
// broadcasts the top-N over the WS hub. Returns when ctx is cancelled.
func Run(ctx context.Context, s store.Store, hub *ws.Hub, topN int, interval time.Duration, log *slog.Logger) {
	scores := make(map[string]int64, len(defaultContestants))
	for _, c := range defaultContestants {
		scores[c.ID] = c.Score
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec

	log.Info("seed-demo active", "contestants", len(scores), "interval", interval)

	flush := func() {
		for id, prev := range scores {
			drift := int64(rng.Intn(driftRange) - driftRange/2)
			next := prev + drift
			switch {
			case next < 0:
				next = 0
			case next > maxScore:
				next = maxScore
			}
			scores[id] = next
			_ = s.Upsert(ctx, id, next)
		}
		top, err := s.Top(ctx, topN)
		if err != nil {
			log.Warn("seed top", "err", err)
			return
		}
		payload, err := json.Marshal(map[string]any{
			"top":        top,
			"at_unix_ms": time.Now().UnixMilli(),
		})
		if err != nil {
			log.Warn("seed marshal", "err", err)
			return
		}
		hub.Broadcast(payload)
	}

	flush() // populate immediately so the UI is never empty

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flush()
		}
	}
}
