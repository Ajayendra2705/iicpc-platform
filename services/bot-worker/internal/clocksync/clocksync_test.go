package clocksync_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/clocksync"
)

func TestEstimateOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/time" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{"now_ns": time.Now().UnixNano()})
	}))
	defer srv.Close()

	offset, err := clocksync.EstimateOffset(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("EstimateOffset: %v", err)
	}
	// offset against a local server should be within 10ms
	if offset < -10_000_000 || offset > 10_000_000 {
		t.Errorf("offset %d ns outside expected ±10ms range", offset)
	}
}

func TestEstimateOffsetUnreachable(t *testing.T) {
	_, err := clocksync.EstimateOffset(context.Background(), http.DefaultClient, "http://127.0.0.1:19999")
	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
}
