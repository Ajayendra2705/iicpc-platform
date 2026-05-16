package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/httpapi"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ingest"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/score"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/store"
	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ws"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	httpAddr := envOr("HTTP_ADDR", ":8086")
	storeKind := envOr("STORE_KIND", "stub")
	aggURL := envOr("AGGREGATOR_URL", "http://localhost:8084")
	valURL := envOr("VALIDATOR_URL", "http://localhost:8085")
	tickMs := envInt("TICK_MS", 1000)
	topN := envInt("TOP_N", 100)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var s store.Store
	switch storeKind {
	case "redis":
		r := store.NewRedis(store.RedisConfig{
			Addr:     envOr("REDIS_ADDR", "localhost:6379"),
			Password: envOr("REDIS_PASSWORD", ""),
			Key:      envOr("REDIS_KEY", "leaderboard:scores"),
		})
		if err := r.Ping(ctx); err != nil {
			log.Error("redis ping", "err", err)
			os.Exit(1)
		}
		s = r
		log.Info("redis store connected")
	default:
		s = store.NewStub()
		log.Info("stub store (no persistence)")
	}

	hub := ws.NewHub()
	ing := &ingest.Ingester{
		Fetcher:  ingest.NewHTTPFetcher(aggURL, valURL),
		Calc:     score.New(score.DefaultConfig()),
		Store:    s,
		Hub:      hub,
		TopN:     topN,
		Interval: time.Duration(tickMs) * time.Millisecond,
		Log:      log,
	}
	go ing.Run(ctx)

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           httpapi.New(s, hub, log).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Info("leaderboard-svc starting", "addr", httpAddr, "store", storeKind, "agg", aggURL, "val", valURL)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http serve", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	_ = s.Close()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
