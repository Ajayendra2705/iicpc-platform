package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/build"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/httpx"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/storage"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/store"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/validation"
)

type Config struct {
	MaxArchiveBytes int64
	Storage         storage.ObjectStore
	Submissions     store.Repository
	Builder         build.Builder
	Logger          *slog.Logger
	InternalToken   string
}

type Server struct {
	cfg     Config
	mux     *http.ServeMux
	handler http.Handler
	log     *slog.Logger
}

func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux(), log: cfg.Logger.With("component", "http")}
	s.routes()
	s.handler = httpx.Chain(s.mux,
		httpx.RequestID,
		httpx.AccessLog(s.log),
		httpx.InternalToken(cfg.InternalToken),
	)
	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
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

	// Sniff first two bytes for gzip magic (1f 8b). Reject non-gzip uploads
	// before we ever push them to object storage.
	br := bufio.NewReader(file)
	magic, err := br.Peek(2)
	if err != nil || len(magic) < 2 || magic[0] != 0x1f || magic[1] != 0x8b {
		writeError(w, http.StatusBadRequest, "archive must be gzip-compressed (.tar.gz)")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	key := fmt.Sprintf("%s/%s/source.tar.gz", contestantID, id)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	uri, err := s.cfg.Storage.Put(ctx, key, br, header.Size, "application/gzip")
	if err != nil {
		s.log.Error("storage put", "request_id", httpx.RequestIDFrom(r.Context()), "err", err)
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
		s.log.Error("create submission", "request_id", httpx.RequestIDFrom(r.Context()), "err", err)
		writeError(w, http.StatusInternalServerError, "persist failed")
		return
	}

	if err := s.cfg.Builder.Enqueue(id); err != nil {
		if errors.Is(err, build.ErrQueueFull) {
			writeError(w, http.StatusServiceUnavailable, "build queue full, retry later")
			return
		}
		s.log.Error("enqueue build", "request_id", httpx.RequestIDFrom(r.Context()), "err", err)
		writeError(w, http.StatusInternalServerError, "enqueue failed")
		return
	}

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
	_ = json.NewEncoder(w).Encode(body)
}
