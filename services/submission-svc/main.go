package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iicpc/platform/services/submission-svc/internal/build"
	httpsrv "github.com/iicpc/platform/services/submission-svc/internal/server"
	"github.com/iicpc/platform/services/submission-svc/internal/storage"
	"github.com/iicpc/platform/services/submission-svc/internal/store"
)

func main() {
	cfg := loadConfig()

	objStore, err := storage.NewMinIO(storage.Config{
		Endpoint:        cfg.minioEndpoint,
		AccessKey:       cfg.minioAccessKey,
		SecretKey:       cfg.minioSecretKey,
		Bucket:          cfg.minioBucket,
		UseSSL:          cfg.minioUseSSL,
		EnsureBucket:    true,
	})
	if err != nil {
		log.Fatalf("init object store: %v", err)
	}

	repo := store.NewMemory()
	builder := build.NewStub(repo)

	go builder.Run(context.Background())

	srv := httpsrv.New(httpsrv.Config{
		MaxArchiveBytes: cfg.maxArchiveBytes,
		Storage:         objStore,
		Submissions:     repo,
		Builder:         builder,
	})

	httpServer := &http.Server{
		Addr:              cfg.httpAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("submission-svc listening on %s", cfg.httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

type config struct {
	httpAddr        string
	maxArchiveBytes int64
	minioEndpoint   string
	minioAccessKey  string
	minioSecretKey  string
	minioBucket     string
	minioUseSSL     bool
}

func loadConfig() config {
	return config{
		httpAddr:        envOr("SUBMISSION_HTTP_ADDR", ":8081"),
		maxArchiveBytes: 50 * 1024 * 1024,
		minioEndpoint:   envOr("MINIO_ENDPOINT", "minio:9000"),
		minioAccessKey:  envOr("MINIO_ACCESS_KEY", "minioadmin"),
		minioSecretKey:  envOr("MINIO_SECRET_KEY", "minioadmin"),
		minioBucket:     envOr("MINIO_BUCKET", "submissions"),
		minioUseSSL:     envOr("MINIO_USE_SSL", "false") == "true",
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
