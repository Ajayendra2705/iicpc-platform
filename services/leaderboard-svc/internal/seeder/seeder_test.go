package seeder_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/seeder"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/store"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ws"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestSeederPopulatesStore(t *testing.T) {
	s := store.NewStub()
	hub := ws.NewHub()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	seeder.Run(ctx, s, hub, 10, 50*time.Millisecond, quietLogger())

	top, _ := s.Top(context.Background(), 100)
	if len(top) < 6 {
		t.Errorf("expected ≥ 6 contestants seeded, got %d", len(top))
	}
	for _, e := range top {
		if e.Score < 0 || e.Score > 1000 {
			t.Errorf("score out of range: %s=%d", e.ContestantID, e.Score)
		}
	}
}

func TestSeederBroadcasts(t *testing.T) {
	s := store.NewStub()
	hub := ws.NewHub()
	c := hub.Register()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	go seeder.Run(ctx, s, hub, 10, 20*time.Millisecond, quietLogger())

	select {
	case <-c.Send():
		// success — first broadcast received within 100ms
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no broadcast in 200ms")
	}
}

func TestSeederStopsOnCancel(t *testing.T) {
	s := store.NewStub()
	hub := ws.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		seeder.Run(ctx, s, hub, 10, 50*time.Millisecond, quietLogger())
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("seeder did not stop on cancel")
	}
}
