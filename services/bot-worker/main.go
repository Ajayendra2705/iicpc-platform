package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/client"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/stats"
)

type config struct {
	httpAddr        string
	targetURL       string
	ordersPerSecond float64
	numWorkers      int
	midPrice        float64
	priceSigma      float64
	cancelRatio     float64
	workerID        string
	protocol        string // rest | ws | fix
	wsPath          string // WebSocket path appended to targetURL
	fixHost         string
	fixPort         int
}

func loadConfig() config {
	return config{
		httpAddr:        envOr("HTTP_ADDR", ":9090"),
		targetURL:       envOr("TARGET_URL", "http://localhost:8082"),
		ordersPerSecond: envFloat("ORDERS_PER_SECOND", 10),
		numWorkers:      envInt("NUM_WORKERS", 1),
		midPrice:        envFloat("MID_PRICE", 100),
		priceSigma:      envFloat("PRICE_SIGMA", 1.0),
		cancelRatio:     envFloat("CANCEL_RATIO", 0.70),
		workerID:        envOr("WORKER_ID", "bot-0"),
		protocol:        envOr("PROTOCOL", "rest"),
		wsPath:          envOr("WS_PATH", "/ws"),
		fixHost:         envOr("FIX_HOST", "localhost"),
		fixPort:         envInt("FIX_PORT", 5001),
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfig()

	rec := stats.New()
	genCfg := gen.Config{
		MidPrice:    cfg.midPrice,
		PriceSigma:  cfg.priceSigma,
		CancelRatio: cfg.cancelRatio,
	}
	interval := time.Duration(float64(time.Second) / cfg.ordersPerSecond)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	factory, err := makeWorkerFactory(ctx, cfg, logger)
	if err != nil {
		logger.Error("build client factory", "err", err)
		os.Exit(1)
	}

	var wg sync.WaitGroup
	for i := 0; i < cfg.numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cli, err := factory(ctx)
			if err != nil {
				logger.Error("create client", "id", id, "err", err)
				return
			}
			defer cli.Close()
			runWorker(ctx, id, cli, gen.New(genCfg), rec, interval, logger)
		}(i)
	}

	srv := buildServer(cfg.httpAddr, rec)
	go func() {
		logger.Info("bot-worker starting",
			"addr", cfg.httpAddr,
			"target", cfg.targetURL,
			"ops", cfg.ordersPerSecond,
			"workers", cfg.numWorkers,
			"worker_id", cfg.workerID,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server", "err", err)
		}
	}()

	<-ctx.Done()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
	wg.Wait()
}

// makeWorkerFactory returns a function that creates an OrderClient for each worker.
// REST/FIX return the same shared client; WS creates a new connection per worker.
func makeWorkerFactory(ctx context.Context, cfg config, log *slog.Logger) (func(context.Context) (client.OrderClient, error), error) {
	switch strings.ToLower(cfg.protocol) {
	case "rest", "":
		cli := client.New(client.Config{BaseURL: cfg.targetURL, Timeout: 5 * time.Second})
		return func(_ context.Context) (client.OrderClient, error) { return cli, nil }, nil

	case "ws":
		wsURL := cfg.targetURL + cfg.wsPath
		return func(ctx context.Context) (client.OrderClient, error) {
			return client.DialWS(ctx, wsURL)
		}, nil

	case "fix":
		cli, err := client.NewFIX(client.FIXConfig{
			TargetHost:   cfg.fixHost,
			TargetPort:   cfg.fixPort,
			SenderCompID: cfg.workerID,
		}, log)
		if err != nil {
			return nil, fmt.Errorf("new FIX client: %w", err)
		}
		if err := cli.Connect(ctx); err != nil {
			return nil, fmt.Errorf("FIX connect: %w", err)
		}
		return func(_ context.Context) (client.OrderClient, error) { return cli, nil }, nil

	default:
		return nil, fmt.Errorf("unknown PROTOCOL %q (want rest|ws|fix)", cfg.protocol)
	}
}

func buildServer(addr string, rec *stats.Recorder) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		snap := rec.Snapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func runWorker(
	ctx context.Context,
	id int,
	cli client.OrderClient,
	g *gen.Generator,
	rec *stats.Recorder,
	interval time.Duration,
	log *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var liveIDs []string
	log.Info("bot worker started", "id", id, "interval", interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// opportunistically cancel a live order
			if len(liveIDs) > 0 && g.ShouldCancel() {
				pick := liveIDs[0]
				liveIDs = liveIDs[1:]
				latNs, err := cli.CancelOrder(ctx, pick)
				if err != nil {
					rec.RecordError()
					log.Warn("cancel failed", "order_id", pick, "err", err)
				} else {
					rec.RecordLatency(latNs)
				}
			}

			// place a new order every tick
			ord := g.Next()
			oid, latNs, err := cli.PlaceOrder(ctx, string(ord.Side), ord.Price, ord.Qty)
			if err != nil {
				rec.RecordError()
				log.Warn("place failed", "err", err)
				continue
			}
			rec.RecordLatency(latNs)
			liveIDs = append(liveIDs, oid)
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
