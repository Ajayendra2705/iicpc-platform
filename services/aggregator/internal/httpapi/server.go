// Package httpapi exposes the aggregator's latest snapshots over HTTP.
package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

type Server struct {
	agg *windowing.Aggregator
}

func New(agg *windowing.Aggregator) *Server {
	return &Server{agg: agg}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleAll)
	mux.HandleFunc("GET /metrics/merged", s.handleMergedAll)
	mux.HandleFunc("GET /metrics/{contestant_id}", s.handleOne)
	mux.HandleFunc("GET /metrics/merged/{contestant_id}", s.handleMerged)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAll(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.agg.All())
}

func (s *Server) handleOne(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("contestant_id")
	snap, ok := s.agg.Latest(id)
	if !ok {
		http.Error(w, "no data for "+id, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

// handleMergedAll returns the whole-run merged snapshot for every contestant.
// ?windows=N bounds the merge to the last N windows; default 0 merges all
// retained history. This is the endpoint the leaderboard scores on so rankings
// reflect a contestant's full run, not the most recent 1-second window.
func (s *Server) handleMergedAll(w http.ResponseWriter, r *http.Request) {
	windows := 0 // 0 → entire retained history
	if v := r.URL.Query().Get("windows"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windows = n
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.agg.AllMerged(windows))
}

// handleMerged returns percentiles correctly merged over the last N windows
// (N from ?windows=N, default 60). Unlike /metrics/{id} which only sees the
// most recent flushed window, this gives a multi-window view with statistically
// correct percentiles — histograms are merged bucket-by-bucket, NOT averaged.
func (s *Server) handleMerged(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("contestant_id")
	windows := 60
	if v := r.URL.Query().Get("windows"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windows = n
		}
	}
	merged, ok := s.agg.MergedRecent(id, windows)
	if !ok {
		http.Error(w, "no history for "+id, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(merged)
}
