package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/api-gateway/internal/auth"
	"github.com/Ajayendra2705/iicpc-platform/services/api-gateway/internal/httpx"
	"github.com/Ajayendra2705/iicpc-platform/services/api-gateway/internal/proxy"
	"github.com/Ajayendra2705/iicpc-platform/services/api-gateway/internal/ratelimit"
)

type config struct {
	Addr             string
	JWTSecret        string
	JWTTokenTTL      time.Duration
	SubmissionSvcURL string
	RateRPS          float64
	RateBurst        float64
}

func loadConfig() config {
	return config{
		Addr:             envOr("API_GATEWAY_ADDR", ":8080"),
		JWTSecret:        envOr("JWT_SECRET", "change-me-in-prod"),
		JWTTokenTTL:      15 * time.Minute,
		SubmissionSvcURL: envOr("SUBMISSION_SVC_URL", "http://localhost:8081"),
		RateRPS:          20,
		RateBurst:        50,
	}
}

func buildHandler(cfg config, submProxy http.Handler) http.Handler {
	rl := ratelimit.New(cfg.RateRPS, cfg.RateBurst)
	jwtMW := func(h http.Handler) http.Handler { return auth.Middleware(cfg.JWTSecret, h) }

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("POST /auth/token", handleToken(cfg))
	mux.Handle("POST /submissions", jwtMW(submProxy))
	mux.Handle("GET /submissions/{id}", jwtMW(submProxy))
	mux.Handle("GET /submissions/{id}/logs", jwtMW(submProxy))
	mux.Handle("GET /submissions", jwtMW(submProxy))

	return httpx.AccessLog(httpx.RequestID(rl.Middleware(mux)))
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := loadConfig()

	submProxy, err := proxy.New(cfg.SubmissionSvcURL)
	if err != nil {
		slog.Error("proxy init failed", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: buildHandler(cfg, submProxy),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		slog.Info("api-gateway starting", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

type tokenReq struct {
	ContestantID string `json:"contestant_id"`
}

type tokenResp struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

func handleToken(cfg config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req tokenReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ContestantID == "" {
			http.Error(w, "contestant_id required", http.StatusBadRequest)
			return
		}
		tok, err := auth.Issue(req.ContestantID, cfg.JWTSecret, cfg.JWTTokenTTL)
		if err != nil {
			http.Error(w, "token issue failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResp{
			Token:     tok,
			ExpiresIn: int(cfg.JWTTokenTTL.Seconds()),
		})
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
