// Package httpapi exposes the leaderboard over HTTP plus a WS live stream.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/store"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ws"
)

type Server struct {
	Store    store.Store
	Hub      *ws.Hub
	Log      *slog.Logger
	upgrader gws.Upgrader
}

func New(s store.Store, h *ws.Hub, log *slog.Logger) *Server {
	return &Server{
		Store: s,
		Hub:   h,
		Log:   log,
		upgrader: gws.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(_ *http.Request) bool { return true }, // demo-friendly
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /leaderboard", s.handleLeaderboard)
	mux.HandleFunc("GET /live", s.handleLive)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	top := 100
	if t := r.URL.Query().Get("top"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			top = n
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	entries, err := s.Store.Top(ctx, top)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Warn("ws upgrade", "err", err)
		return
	}
	client := s.Hub.Register()
	defer s.Hub.Unregister(client)
	defer conn.Close()

	// reader: discard incoming frames; close on error
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// writer: pump hub messages onto the WS until client channel closes
	for msg := range client.Send() {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(gws.TextMessage, msg); err != nil {
			return
		}
	}
}
