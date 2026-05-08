package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
		Endpoint:     cfg.minioEndpoint,
		AccessKey:    cfg.minioAccessKey,
		SecretKey:    cfg.minioSecretKey,
		Bucket:       cfg.minioBucket,
		UseSSL:       cfg.minioUseSSL,
		EnsureBucket: true,
	})
	if err != nil {
		log.Fatalf("init object store: %v", err)
	}

	repo := store.NewMemory()

	builder, builderRun := newBuilder(cfg, repo, objStore)
	go builderRun(context.Background())

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
		log.Printf("submission-svc listening on %s (builder=%s)", cfg.httpAddr, cfg.builderKind)
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

func newBuilder(cfg config, repo store.Repository, objStore storage.ObjectStore) (build.Builder, func(context.Context)) {
	switch strings.ToLower(cfg.builderKind) {
	case "buildkit":
		bk := build.NewBuildKit(build.BuildKitConfig{
			Repo:            repo,
			Storage:         objStore,
			SandboxImageDir: cfg.sandboxImageDir,
			RegistryAddr:    cfg.registryAddr,
			ImageRepoPrefix: "contestants",
			BuildTimeout:    cfg.buildTimeout,
		})
		return bk, bk.Run
	case "stub", "":
		s := build.NewStub(repo)
		return s, s.Run
	default:
		log.Fatalf("unknown BUILDER_KIND %q (want stub|buildkit)", cfg.builderKind)
		return nil, nil
	}
}

type config struct {
	httpAddr        string
	maxArchiveBytes int64
	minioEndpoint   string
	minioAccessKey  string
	minioSecretKey  string
	minioBucket     string
	minioUseSSL     bool
	builderKind     string
	sandboxImageDir string
	registryAddr    string
	buildTimeout    time.Duration
}

func loadConfig() config {
	timeout, _ := time.ParseDuration(envOr("BUILD_TIMEOUT", "5m"))
	return config{
		httpAddr:        envOr("SUBMISSION_HTTP_ADDR", ":8081"),
		maxArchiveBytes: 50 * 1024 * 1024,
		minioEndpoint:   envOr("MINIO_ENDPOINT", "minio:9000"),
		minioAccessKey:  envOr("MINIO_ACCESS_KEY", "minioadmin"),
		minioSecretKey:  envOr("MINIO_SECRET_KEY", "minioadmin"),
		minioBucket:     envOr("MINIO_BUCKET", "submissions"),
		minioUseSSL:     envOr("MINIO_USE_SSL", "false") == "true",
		builderKind:     envOr("BUILDER_KIND", "stub"),
		sandboxImageDir: defaultSandboxDir(),
		registryAddr:    envOr("REGISTRY_ADDR", "localhost:5000"),
		buildTimeout:    timeout,
	}
}

func defaultSandboxDir() string {
	if v := os.Getenv("SANDBOX_IMAGE_DIR"); v != "" {
		return v
	}
	// When running `go run ./services/submission-svc` from the repo root, the
	// CWD is the repo root, so the sandbox-images directory sits next to it.
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "sandbox-images")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "/sandbox-images"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
