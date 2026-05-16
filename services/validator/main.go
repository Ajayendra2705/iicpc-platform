package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/consumer"
	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/httpapi"
	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/replay"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	httpAddr := envOr("HTTP_ADDR", ":8085")
	consumerKind := envOr("CONSUMER_KIND", "stub")

	var cons consumer.Consumer
	switch consumerKind {
	case "kafka":
		brokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:9092"), ",")
		topic := envOr("KAFKA_TOPIC", "telemetry-events")
		groupID := envOr("KAFKA_GROUP_ID", "validator")
		cons = consumer.NewKafka(consumer.KafkaConfig{Brokers: brokers, Topic: topic, GroupID: groupID}, log)
		log.Info("kafka consumer", "brokers", brokers, "topic", topic, "group", groupID)
	default:
		cons = consumer.NewStub(nil)
		log.Info("stub consumer (no upstream)")
	}

	v := replay.NewValidator()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := cons.Consume(ctx, v.Process); err != nil {
			log.Error("consumer", "err", err)
		}
	}()

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           httpapi.New(v).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Info("validator starting", "addr", httpAddr, "consumer", consumerKind)

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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
