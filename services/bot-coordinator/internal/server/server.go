package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-coordinator/internal/spawn"
)

type Server struct {
	spawner spawn.JobSpawner
	log     *slog.Logger
}

func New(spawner spawn.JobSpawner, log *slog.Logger) *Server {
	return &Server{spawner: spawner, log: log}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /benchmarks", s.handleStart)
	mux.HandleFunc("GET /benchmarks/{id}", s.handleStatus)
	mux.HandleFunc("DELETE /benchmarks/{id}", s.handleStop)
	return mux
}

type startReq struct {
	spawn.BenchmarkSpec
	BenchmarkID string `json:"benchmark_id"`
}

type startResp struct {
	BenchmarkID string `json:"benchmark_id"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var req startReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.BenchmarkID == "" || strings.ContainsAny(req.BenchmarkID, " /\t\n") {
		http.Error(w, "benchmark_id required (no spaces or slashes)", http.StatusBadRequest)
		return
	}
	if req.TargetURL == "" {
		http.Error(w, "target_url required", http.StatusBadRequest)
		return
	}
	if req.NumWorkers <= 0 {
		req.NumWorkers = 1
	}
	if req.Protocol == "" {
		req.Protocol = "rest"
	}
	if req.ArrivalMode == "" {
		req.ArrivalMode = "poisson"
	}

	if err := s.spawner.Start(r.Context(), req.BenchmarkID, req.BenchmarkSpec); err != nil {
		s.log.Error("spawn start", "benchmark_id", req.BenchmarkID, "err", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(startResp{BenchmarkID: req.BenchmarkID})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status, err := s.spawner.Status(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.spawner.Stop(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
