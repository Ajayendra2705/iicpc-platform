// Package httpapi exposes the aggregator's latest snapshots over HTTP.
package httpapi

import (
	"encoding/json"
	"net/http"

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
	mux.HandleFunc("GET /metrics/{contestant_id}", s.handleOne)
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
