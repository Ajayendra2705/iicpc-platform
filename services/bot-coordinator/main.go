package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-coordinator/internal/server"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-coordinator/internal/spawn"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	httpAddr := envOr("BOT_COORDINATOR_ADDR", ":8083")
	spawnerKind := envOr("SPAWNER_KIND", "stub")

	var spawner spawn.JobSpawner
	if spawnerKind == "k8s" {
		s, err := spawn.NewK8sSpawner(spawn.K8sConfig{
			Namespace:      envOr("K8S_NAMESPACE", "iicpc"),
			BotWorkerImage: envOr("BOT_WORKER_IMAGE", "localhost:5000/bot-worker:latest"),
			KubeConfigPath: envOr("KUBECONFIG", ""),
		})
		if err != nil {
			log.Error("k8s spawner init", "err", err)
			os.Exit(1)
		}
		spawner = s
	} else {
		spawner = spawn.NewStub()
	}

	log.Info("bot-coordinator starting", "addr", httpAddr, "spawner", spawnerKind)

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           server.New(spawner, log).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
