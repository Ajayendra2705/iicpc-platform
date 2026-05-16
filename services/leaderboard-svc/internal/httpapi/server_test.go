package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/httpapi"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/store"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ws"
)

func setup(t *testing.T) http.Handler {
	t.Helper()
	s := store.NewStub()
	ctx := context.Background()
	_ = s.Upsert(ctx, "alpha", 950)
	_ = s.Upsert(ctx, "beta", 700)
	_ = s.Upsert(ctx, "gamma", 850)
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return httpapi.New(s, ws.NewHub(), log).Handler()
}

func TestHealthz(t *testing.T) {
	h := setup(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Errorf("healthz: got %d", w.Code)
	}
}

func TestLeaderboardOrderedByScoreDesc(t *testing.T) {
	h := setup(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/leaderboard", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("leaderboard: got %d", w.Code)
	}
	var entries []store.Entry
	_ = json.NewDecoder(w.Body).Decode(&entries)
	want := []string{"alpha", "gamma", "beta"}
	if len(entries) != 3 {
		t.Fatalf("entries: got %d want 3", len(entries))
	}
	for i, e := range entries {
		if e.ContestantID != want[i] {
			t.Errorf("[%d]: got %s want %s", i, e.ContestantID, want[i])
		}
	}
}

func TestLeaderboardTopParam(t *testing.T) {
	h := setup(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/leaderboard?top=2", nil))
	var entries []store.Entry
	_ = json.NewDecoder(w.Body).Decode(&entries)
	if len(entries) != 2 {
		t.Errorf("top=2: got %d", len(entries))
	}
}

func TestLeaderboardInvalidTopFallsBackToDefault(t *testing.T) {
	h := setup(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/leaderboard?top=garbage", nil))
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}
