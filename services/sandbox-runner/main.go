package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	sandboxv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/sandbox/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/sandbox-runner/internal/runner"
	"github.com/Ajayendra2705/iicpc-platform/services/sandbox-runner/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := runner.Config{
		Namespace:        getEnv("K8S_NAMESPACE", "iicpc-contestants"),
		RuntimeClass:     getEnv("RUNTIME_CLASS", "gvisor"),
		CPURequest:       getEnv("CPU_REQUEST", "500m"),
		CPULimit:         getEnv("CPU_LIMIT", "1000m"),
		MemoryRequest:    getEnv("MEMORY_REQUEST", "256Mi"),
		MemoryLimit:      getEnv("MEMORY_LIMIT", "512Mi"),
		EphemeralLimit:   getEnv("EPHEMERAL_LIMIT", "2Gi"),
		ReadinessTimeout: mustDuration(getEnv("READINESS_TIMEOUT", "60s")),
	}

	r, err := runner.New(cfg)
	if err != nil {
		slog.Error("init runner", "err", err)
		os.Exit(1)
	}

	addr := getEnv("GRPC_ADDR", ":9090")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen", "addr", addr, "err", err)
		os.Exit(1)
	}

	gs := grpc.NewServer()
	sandboxv1.RegisterSandboxRunnerServer(gs, server.New(r))
	reflection.Register(gs)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		slog.Info("sandbox-runner listening", "addr", addr)
		if err := gs.Serve(lis); err != nil {
			slog.Error("serve", "err", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutdown signal received")
	gs.GracefulStop()
	slog.Info("stopped")
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		panic("invalid duration: " + s)
	}
	return d
}
