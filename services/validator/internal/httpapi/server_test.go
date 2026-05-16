package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/httpapi"
	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/replay"
)

func setup() http.Handler {
	v := replay.NewValidator()
	v.Process(&telemetryv1.OrderEvent{
		ContestantId: "c1", OrderId: "s1",
		Type:  telemetryv1.OrderType_ORDER_TYPE_LIMIT,
		Price: 99, Quantity: 5, FilledQuantity: 0, SentTsNs: 1,
	})
	v.Process(&telemetryv1.OrderEvent{
		ContestantId: "c1", OrderId: "b1",
		Type:  telemetryv1.OrderType_ORDER_TYPE_LIMIT,
		Price: 100, Quantity: 5, FilledQuantity: 5, SentTsNs: 2,
	})
	return httpapi.New(v).Handler()
}

func TestHealthz(t *testing.T) {
	h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Errorf("healthz: got %d", w.Code)
	}
}

func TestGetAll(t *testing.T) {
	h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/validate", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("validate: got %d", w.Code)
	}
	var reps []replay.Report
	_ = json.NewDecoder(w.Body).Decode(&reps)
	if len(reps) != 1 || reps[0].ContestantID != "c1" {
		t.Errorf("reports: %+v", reps)
	}
}

func TestGetOne(t *testing.T) {
	h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/validate/c1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("validate/c1: got %d", w.Code)
	}
	var r replay.Report
	_ = json.NewDecoder(w.Body).Decode(&r)
	if r.Correctness != 1.0 {
		t.Errorf("correctness: got %v want 1.0", r.Correctness)
	}
}

func TestGetOneNotFound(t *testing.T) {
	h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/validate/nope", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("not found: got %d", w.Code)
	}
}
