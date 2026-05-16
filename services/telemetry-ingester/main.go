package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/telemetry-ingester/internal/buffer"
	"github.com/Ajayendra2705/iicpc-platform/services/telemetry-ingester/internal/producer"
	"github.com/Ajayendra2705/iicpc-platform/services/telemetry-ingester/internal/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	grpcAddr := envOr("GRPC_ADDR", ":9091")
	bufCap := envInt("BUFFER_CAPACITY", 16384)
	batchSize := envInt("BATCH_SIZE", 256)
	flushMs := envInt("FLUSH_INTERVAL_MS", 100)
	prodKind := envOr("PRODUCER_KIND", "stub")

	var prod producer.Producer
	switch prodKind {
	case "kafka":
		brokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:9092"), ",")
		topic := envOr("KAFKA_TOPIC", "telemetry-events")
		prod = producer.NewKafka(producer.KafkaConfig{Brokers: brokers, Topic: topic})
		log.Info("kafka producer", "brokers", brokers, "topic", topic)
	default:
		prod = producer.NewStub()
		log.Info("stub producer (no downstream)")
	}

	buf := buffer.New(bufCap)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	drainDone := make(chan struct{})
	go func() {
		drainLoop(ctx, buf, prod, batchSize, time.Duration(flushMs)*time.Millisecond, log)
		close(drainDone)
	}()

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Error("listen", "addr", grpcAddr, "err", err)
		os.Exit(1)
	}

	gs := grpc.NewServer()
	telemetryv1.RegisterTelemetryIngesterServer(gs, server.New(buf, log))
	reflection.Register(gs)

	log.Info("telemetry-ingester starting",
		"addr", grpcAddr,
		"buffer_capacity", bufCap,
		"batch_size", batchSize,
		"flush_ms", flushMs,
		"producer", prodKind,
	)

	go func() {
		if err := gs.Serve(lis); err != nil {
			log.Error("grpc serve", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutdown signal")

	gs.GracefulStop()
	buf.Close()
	<-drainDone
	_ = prod.Close()
	log.Info("stopped", "stats", buf.Stats())
}

// drainLoop reads from buf.Recv(), accumulates a batch, and publishes it
// whenever the batch fills or flushInterval elapses. Exits when ctx is done
// AND the buffer channel is fully drained.
func drainLoop(ctx context.Context, buf *buffer.Buffer, prod producer.Producer, batchSize int, flushInterval time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	pending := make([]*telemetryv1.OrderEvent, 0, batchSize)
	publishCtx := context.Background()

	flush := func() {
		if len(pending) == 0 {
			return
		}
		if err := prod.Publish(publishCtx, pending); err != nil {
			log.Error("publish batch", "err", err, "size", len(pending))
		}
		pending = pending[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// drain remaining events synchronously
			for e := range buf.Recv() {
				pending = append(pending, e)
				if len(pending) >= batchSize {
					flush()
				}
			}
			flush()
			return
		case <-ticker.C:
			flush()
		case e, ok := <-buf.Recv():
			if !ok {
				flush()
				return
			}
			pending = append(pending, e)
			if len(pending) >= batchSize {
				flush()
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
