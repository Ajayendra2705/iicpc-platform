package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/iicpc/platform/services/submission-svc/internal/build"
	"github.com/iicpc/platform/services/submission-svc/internal/storage"
	"github.com/iicpc/platform/services/submission-svc/internal/store"
	"github.com/iicpc/platform/services/submission-svc/internal/validation"
)

type Config struct {
	MaxArchiveBytes int64
	Storage         storage.ObjectStore
	Submissions     store.Repository
	Builder         build.Builder
}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.healthz)
	s.mux.HandleFunc("POST /submissions", s.createSubmission)
	s.mux.HandleFunc("GET /submissions/{id}", s.getSubmission)
	s.mux.HandleFunc("GET /submissions", s.listSubmissions)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) createSubmission(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxArchiveBytes+1024*1024)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "archive exceeds max size")
			return
		}
		writeError(w, http.StatusBadRequest, "parse multipart: "+err.Error())
		return
	}

	contestantID := r.FormValue("contestant_id")
	entrypoint := r.FormValue("entrypoint")
	lang := store.Language(r.FormValue("language"))

	if err := validation.ValidateRequest(contestantID, entrypoint, lang); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	file, header, err := r.FormFile("archive")
	if err != nil {
		writeError(w, http.StatusBadRequest, "archive form file required")
		return
	}
	defer file.Close()

	if header.Size > s.cfg.MaxArchiveBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "archive exceeds max size")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	key := fmt.Sprintf("%s/%s/source.tar.gz", contestantID, id)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	uri, err := s.cfg.Storage.Put(ctx, key, file, header.Size, "application/gzip")
	if err != nil {
		log.Printf("storage put: %v", err)
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	sub := store.Submission{
		ID:           id,
		ContestantID: contestantID,
		Language:     lang,
		Status:       store.StatusUploaded,
		SourceURI:    uri,
		Entrypoint:   entrypoint,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.cfg.Submissions.Create(sub); err != nil {
		log.Printf("create submission: %v", err)
		writeError(w, http.StatusInternalServerError, "persist failed")
		return
	}

	s.cfg.Builder.Enqueue(id)

	writeJSON(w, http.StatusAccepted, sub)
}

func (s *Server) getSubmission(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sub, ok := s.cfg.Submissions.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "submission not found")
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

func (s *Server) listSubmissions(w http.ResponseWriter, r *http.Request) {
	contestantID := r.URL.Query().Get("contestant_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	subs := s.cfg.Submissions.List(contestantID, limit)
	writeJSON(w, http.StatusOK, map[string]any{"submissions": subs})
}

type errorResp struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResp{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encode response: %v", err)
	}
}
