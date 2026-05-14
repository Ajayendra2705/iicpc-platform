package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/build"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/sandbox"
	httpsrv "github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/server"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/storage"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

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
		logger.Error("init object store", "err", err)
		os.Exit(1)
	}

	repo, repoClose, err := newRepo(cfg, logger)
	if err != nil {
		logger.Error("init repository", "err", err)
		os.Exit(1)
	}
	defer repoClose()

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	var sandboxStarter build.SandboxStarter
	if cfg.sandboxRunnerAddr != "" {
		gr, err := sandbox.NewGRPC(cfg.sandboxRunnerAddr)
		if err != nil {
			logger.Error("init sandbox runner client", "err", err)
			os.Exit(1)
		}
		defer gr.Close()
		sandboxStarter = gr
		logger.Info("sandbox runner configured", "addr", cfg.sandboxRunnerAddr)
	}

	builder, builderRun := newBuilder(cfg, repo, objStore, logger, sandboxStarter)

	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		builderRun(rootCtx)
	}()

	srv := httpsrv.New(httpsrv.Config{
		MaxArchiveBytes: cfg.maxArchiveBytes,
		Storage:         objStore,
		Submissions:     repo,
		Builder:         builder,
		Logger:          logger,
		InternalToken:   cfg.internalToken,
	})

	httpServer := &http.Server{
		Addr:              cfg.httpAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("submission-svc listening",
			"addr", cfg.httpAddr,
			"builder", cfg.builderKind,
			"store", cfg.storeKind)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)

	// Cancel in-flight builds, then wait for the worker goroutine to exit.
	rootCancel()
	workerDone := make(chan struct{})
	go func() {
		workerWG.Wait()
		close(workerDone)
	}()
	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
		logger.Warn("worker drain timed out")
	}
}

func newRepo(cfg config, log *slog.Logger) (store.Repository, func(), error) {
	switch strings.ToLower(cfg.storeKind) {
	case "postgres":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pg, err := store.NewPostgres(ctx, cfg.storeDSN)
		if err != nil {
			return nil, nil, err
		}
		log.Info("repository: postgres")
		return pg, pg.Close, nil
	case "memory", "":
		log.Info("repository: memory (state lost on restart)")
		return store.NewMemory(), func() {}, nil
	default:
		return nil, nil, errors.New("unknown STORE_KIND " + cfg.storeKind)
	}
}

func newBuilder(cfg config, repo store.Repository, objStore storage.ObjectStore, log *slog.Logger, sb build.SandboxStarter) (build.Builder, func(context.Context)) {
	switch strings.ToLower(cfg.builderKind) {
	case "buildkit":
		var scanner build.ImageScanner
		if cfg.trivyEnabled {
			if ts := build.NewTrivyScanner(log); ts != nil {
				scanner = ts
			} else {
				log.Warn("TRIVY_ENABLED=true but trivy not on PATH; scanning disabled")
			}
		}
		bk := build.NewBuildKit(build.BuildKitConfig{
			Repo:            repo,
			Storage:         objStore,
			Logger:          log,
			SandboxImageDir: cfg.sandboxImageDir,
			RegistryAddr:    cfg.registryAddr,
			ImageRepoPrefix: "contestants",
			BuildTimeout:    cfg.buildTimeout,
			Scanner:         scanner,
			Sandbox:         sb,
		})
		return bk, bk.Run
	case "stub", "":
		s := build.NewStubWithSandbox(repo, log, sb)
		return s, s.Run
	default:
		log.Error("unknown BUILDER_KIND", "value", cfg.builderKind)
		os.Exit(1)
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
	storeKind       string
	storeDSN        string
	internalToken      string
	trivyEnabled       bool
	sandboxRunnerAddr  string
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
		storeKind:       envOr("STORE_KIND", "memory"),
		storeDSN:        envOr("STORE_DSN", "postgres://iicpc:iicpc@localhost:5432/iicpc?sslmode=disable"),
		internalToken:     os.Getenv("INTERNAL_TOKEN"),
		trivyEnabled:      envOr("TRIVY_ENABLED", "false") == "true",
		sandboxRunnerAddr: os.Getenv("SANDBOX_RUNNER_ADDR"),
	}
}

func defaultSandboxDir() string {
	if v := os.Getenv("SANDBOX_IMAGE_DIR"); v != "" {
		return v
	}
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
