package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-coordinator/internal/server"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-coordinator/internal/spawn"
	"log/slog"
	"os"
)

func newSrv() http.Handler {
	return server.New(spawn.NewStub(), slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))).Handler()
}

func TestHealthz(t *testing.T) {
	h := newSrv()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("healthz: got %d", w.Code)
	}
}

func TestStartBenchmark(t *testing.T) {
	h := newSrv()
	body, _ := json.Marshal(map[string]any{
		"benchmark_id":      "bench-1",
		"target_url":        "http://target:8082",
		"num_workers":       3,
		"orders_per_second": 50,
		"protocol":          "rest",
		"arrival_mode":      "poisson",
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/benchmarks", bytes.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("start: got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["benchmark_id"] != "bench-1" {
		t.Errorf("benchmark_id: got %q want bench-1", resp["benchmark_id"])
	}
}

func TestStartDuplicateBenchmark(t *testing.T) {
	h := newSrv()
	body, _ := json.Marshal(map[string]any{
		"benchmark_id": "dup",
		"target_url":   "http://x",
	})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/benchmarks", bytes.NewReader(body)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/benchmarks", bytes.NewReader(body)))
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate start: got %d want 409", w.Code)
	}
}

func TestStartMissingBenchmarkID(t *testing.T) {
	h := newSrv()
	body, _ := json.Marshal(map[string]any{"target_url": "http://x"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/benchmarks", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing id: got %d want 400", w.Code)
	}
}

func TestStartMissingTargetURL(t *testing.T) {
	h := newSrv()
	body, _ := json.Marshal(map[string]any{"benchmark_id": "b1"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/benchmarks", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing url: got %d want 400", w.Code)
	}
}

func TestGetStatus(t *testing.T) {
	h := newSrv()
	body, _ := json.Marshal(map[string]any{
		"benchmark_id": "b1",
		"target_url":   "http://x",
		"num_workers":  4,
	})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/benchmarks", bytes.NewReader(body)))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/benchmarks/b1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d", w.Code)
	}
	var st spawn.JobStatus
	_ = json.NewDecoder(w.Body).Decode(&st)
	if st.Active != 4 {
		t.Errorf("active: got %d want 4", st.Active)
	}
}

func TestGetStatusNotFound(t *testing.T) {
	h := newSrv()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/benchmarks/nope", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status not found: got %d want 404", w.Code)
	}
}

func TestStopBenchmark(t *testing.T) {
	h := newSrv()
	body, _ := json.Marshal(map[string]any{"benchmark_id": "b1", "target_url": "http://x"})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/benchmarks", bytes.NewReader(body)))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/benchmarks/b1", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("stop: got %d want 204", w.Code)
	}
}

func TestStopNotFound(t *testing.T) {
	h := newSrv()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/benchmarks/nope", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("stop not found: got %d want 404", w.Code)
	}
}
