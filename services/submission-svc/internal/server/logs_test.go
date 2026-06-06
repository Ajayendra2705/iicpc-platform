package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/buildlog"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/store"
)

func TestGetSubmissionLogs(t *testing.T) {
	repo := store.NewMemory()
	_ = repo.Create(store.Submission{ID: "sub-1", ContestantID: "team", Language: store.LangGo})
	logs := buildlog.NewBuffer(0)
	logs.Append("sub-1", "status", "Build started")
	logs.Append("sub-1", "stdout", "Step 1/3 : FROM golang")

	srv := New(Config{
		Submissions: repo,
		Logs:        logs,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// Full fetch.
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/submissions/sub-1/logs", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d", w.Code)
	}
	var resp struct {
		Lines  []buildlog.Line `json:"lines"`
		Latest int64           `json:"latest"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Lines) != 2 || resp.Latest != 2 {
		t.Fatalf("logs: got %d lines latest=%d, want 2/2", len(resp.Lines), resp.Latest)
	}

	// Incremental fetch via ?since.
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/submissions/sub-1/logs?since=1", nil))
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Lines) != 1 || resp.Lines[0].Seq != 2 {
		t.Fatalf("since=1: got %+v", resp.Lines)
	}
}

func TestGetSubmissionLogsUnknownSubmission(t *testing.T) {
	srv := New(Config{
		Submissions: store.NewMemory(),
		Logs:        buildlog.NewBuffer(0),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/submissions/nope/logs", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", w.Code)
	}
}
