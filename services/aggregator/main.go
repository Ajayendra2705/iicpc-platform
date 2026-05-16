package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/consumer"
	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/httpapi"
	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	httpAddr := envOr("HTTP_ADDR", ":8084")
	consumerKind := envOr("CONSUMER_KIND", "stub")
	windowMs := envInt("WINDOW_MS", 1000)

	var cons consumer.Consumer
	switch consumerKind {
	case "kafka":
		brokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:9092"), ",")
		topic := envOr("KAFKA_TOPIC", "telemetry-events")
		groupID := envOr("KAFKA_GROUP_ID", "aggregator")
		cons = consumer.NewKafka(consumer.KafkaConfig{Brokers: brokers, Topic: topic, GroupID: groupID}, log)
		log.Info("kafka consumer", "brokers", brokers, "topic", topic, "group", groupID)
	default:
		cons = consumer.NewStub(nil)
		log.Info("stub consumer (no upstream)")
	}

	agg := windowing.New(time.Duration(windowMs) * time.Millisecond)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := cons.Consume(ctx, agg.Record); err != nil {
			log.Error("consumer", "err", err)
		}
	}()

	go tickLoop(ctx, agg, time.Duration(windowMs)*time.Millisecond, log)

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           httpapi.New(agg).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Info("aggregator starting", "addr", httpAddr, "window_ms", windowMs, "consumer", consumerKind)

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
	_ = cons.Close()
}

func tickLoop(ctx context.Context, agg *windowing.Aggregator, window time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(window)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			snaps := agg.Flush(t)
			for _, s := range snaps {
				log.Info("window flushed",
					"contestant_id", s.ContestantID,
					"count", s.Count,
					"tps", s.TPS,
					"p50_ns", s.P50Ns,
					"p99_ns", s.P99Ns,
					"errors", s.Rejected+s.Timeouts,
				)
			}
		}
	}
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
