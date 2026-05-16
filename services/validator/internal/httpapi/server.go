// Package httpapi exposes correctness reports over HTTP.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/replay"
)

type Server struct {
	v *replay.Validator
}

func New(v *replay.Validator) *Server { return &Server{v: v} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /validate", s.handleAll)
	mux.HandleFunc("GET /validate/{contestant_id}", s.handleOne)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAll(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.v.All())
}

func (s *Server) handleOne(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("contestant_id")
	rep, ok := s.v.Report(id)
	if !ok {
		http.Error(w, "no data for "+id, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rep)
}
