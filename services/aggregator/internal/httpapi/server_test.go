package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/httpapi"
	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

func setup() (*windowing.Aggregator, http.Handler) {
	agg := windowing.New(time.Second)
	agg.Record(&telemetryv1.OrderEvent{ContestantId: "c1", LatencyNs: 1_000_000, Result: telemetryv1.OrderResult_ORDER_RESULT_FILLED})
	agg.Record(&telemetryv1.OrderEvent{ContestantId: "c2", LatencyNs: 2_000_000, Result: telemetryv1.OrderResult_ORDER_RESULT_FILLED})
	agg.Flush(time.Now())
	return agg, httpapi.New(agg).Handler()
}

func TestHealthz(t *testing.T) {
	_, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Errorf("healthz: got %d", w.Code)
	}
}

func TestGetAllMetrics(t *testing.T) {
	_, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("metrics: got %d", w.Code)
	}
	var snaps []windowing.Snapshot
	if err := json.NewDecoder(w.Body).Decode(&snaps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(snaps) != 2 {
		t.Errorf("metrics: got %d snapshots, want 2", len(snaps))
	}
}

func TestGetOneMetric(t *testing.T) {
	_, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics/c1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("metrics/c1: got %d", w.Code)
	}
	var s windowing.Snapshot
	if err := json.NewDecoder(w.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.ContestantID != "c1" {
		t.Errorf("contestant_id: got %q want c1", s.ContestantID)
	}
}

func TestGetOneMetricNotFound(t *testing.T) {
	_, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics/nope", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("not found: got %d want 404", w.Code)
	}
}

func TestGetMergedMetric(t *testing.T) {
	_, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics/merged/c1?windows=10", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("merged: got %d", w.Code)
	}
	var m windowing.MergedSnapshot
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.ContestantID != "c1" || m.WindowCount < 1 {
		t.Errorf("merged shape: %+v", m)
	}
}

func TestGetMergedMetricNotFound(t *testing.T) {
	_, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics/merged/nope", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d want 404", w.Code)
	}
}
