package main

import (
	"context"
	"encoding/json"
	"errors"
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

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/client"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/clocksync"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/gen"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/stats"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/telemetry"
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
	arrivalMode     string // uniform | poisson
	jitterMs        int    // max uniform jitter per arrival (0 = disabled)
	burstEnabled    bool   // enable periodic 10× burst
	burstMultiplier int    // rate multiplier during burst
	burstEveryS     int    // seconds between burst events
	burstDurationMs int    // burst duration in milliseconds
	telemetryAddr   string // telemetry-ingester gRPC address (empty = disabled)
	telemetryBuffer int    // telemetry client queue size
	contestantID    string // contestant_id tag on emitted events
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
		arrivalMode:     envOr("ARRIVAL_MODE", "uniform"),
		jitterMs:        envInt("JITTER_MS", 0),
		burstEnabled:    envOr("BURST_ENABLED", "false") == "true",
		burstMultiplier: envInt("BURST_MULTIPLIER", 10),
		burstEveryS:     envInt("BURST_EVERY_S", 30),
		burstDurationMs: envInt("BURST_DURATION_MS", 100),
		telemetryAddr:   envOr("TELEMETRY_ADDR", ""),
		telemetryBuffer: envInt("TELEMETRY_BUFFER", 1024),
		contestantID:    envOr("CONTESTANT_ID", "unknown"),
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	arrivals := gen.NewArrivals(gen.ArrivalMode(cfg.arrivalMode), cfg.ordersPerSecond)
	if cfg.jitterMs > 0 {
		arrivals = arrivals.WithJitter(time.Duration(cfg.jitterMs) * time.Millisecond)
	}
	if cfg.burstEnabled {
		arrivals.StartBurst(ctx,
			time.Duration(cfg.burstEveryS)*time.Second,
			time.Duration(cfg.burstDurationMs)*time.Millisecond,
			cfg.burstMultiplier,
		)
	}

	// best-effort clock offset probe; non-fatal if target has no /time endpoint
	probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
	if offset, err := clocksync.EstimateOffset(probeCtx, &http.Client{Timeout: 3 * time.Second}, cfg.targetURL); err != nil {
		logger.Warn("clock probe failed", "err", err)
	} else {
		rec.SetClockOffset(offset)
		logger.Info("clock offset estimated", "offset_ns", offset)
	}
	probeCancel()

	factory, err := makeWorkerFactory(ctx, cfg, logger)
	if err != nil {
		logger.Error("build client factory", "err", err)
		os.Exit(1)
	}

	var tele telemetry.Client = telemetry.NewStub()
	if cfg.telemetryAddr != "" {
		c, err := telemetry.Dial(ctx, cfg.telemetryAddr, cfg.telemetryBuffer, logger)
		if err != nil {
			logger.Warn("telemetry disabled", "err", err)
		} else {
			tele = c
			logger.Info("telemetry stream", "addr", cfg.telemetryAddr, "contestant_id", cfg.contestantID)
		}
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
			runWorker(ctx, id, cli, gen.New(genCfg), rec, arrivals, tele, cfg.contestantID, cfg.workerID, logger)
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
			"arrival_mode", cfg.arrivalMode,
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
	_ = tele.Close()
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
	mux.HandleFunc("GET /time", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{"now_ns": time.Now().UnixNano()})
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
	arrivals *gen.Arrivals,
	tele telemetry.Client,
	contestantID, botID string,
	log *slog.Logger,
) {
	timer := time.NewTimer(arrivals.Next())
	defer timer.Stop()

	var liveIDs []string
	log.Info("bot worker started", "id", id)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			timer.Reset(arrivals.Next())

			// opportunistically cancel a live order
			if len(liveIDs) > 0 && g.ShouldCancel() {
				pick := liveIDs[0]
				liveIDs = liveIDs[1:]
				start := time.Now()
				latNs, err := cli.CancelOrder(ctx, pick)
				switch {
				case err == nil:
					rec.RecordLatency(latNs)
					tele.Emit(&telemetryv1.OrderEvent{
						ContestantId: contestantID, BotId: botID, OrderId: pick,
						Type:     telemetryv1.OrderType_ORDER_TYPE_CANCEL,
						Result:   telemetryv1.OrderResult_ORDER_RESULT_ACK_ONLY,
						SentTsNs: start.UnixNano(), AckTsNs: start.Add(time.Duration(latNs)).UnixNano(),
						LatencyNs: latNs,
					})
				case isShutdownErr(ctx, err):
					return
				default:
					rec.RecordError()
					log.Warn("cancel failed", "order_id", pick, "err", err)
					tele.Emit(&telemetryv1.OrderEvent{
						ContestantId: contestantID, BotId: botID, OrderId: pick,
						Type:     telemetryv1.OrderType_ORDER_TYPE_CANCEL,
						Result:   telemetryv1.OrderResult_ORDER_RESULT_REJECTED,
						SentTsNs: start.UnixNano(),
					})
				}
			}

			// place a new order on every arrival
			ord := g.Next()
			start := time.Now()
			oid, latNs, err := cli.PlaceOrder(ctx, string(ord.Side), ord.Price, ord.Qty)
			switch {
			case err == nil:
				rec.RecordLatency(latNs)
				liveIDs = append(liveIDs, oid)
				tele.Emit(&telemetryv1.OrderEvent{
					ContestantId: contestantID, BotId: botID, OrderId: oid,
					Type:     telemetryv1.OrderType_ORDER_TYPE_LIMIT,
					Result:   telemetryv1.OrderResult_ORDER_RESULT_ACK_ONLY,
					SentTsNs: start.UnixNano(), AckTsNs: start.Add(time.Duration(latNs)).UnixNano(),
					LatencyNs: latNs, Price: ord.Price, Quantity: int64(ord.Qty),
				})
			case isShutdownErr(ctx, err):
				return
			default:
				rec.RecordError()
				log.Warn("place failed", "err", err)
				tele.Emit(&telemetryv1.OrderEvent{
					ContestantId: contestantID, BotId: botID,
					Type:     telemetryv1.OrderType_ORDER_TYPE_LIMIT,
					Result:   telemetryv1.OrderResult_ORDER_RESULT_REJECTED,
					SentTsNs: start.UnixNano(),
					Price:    ord.Price, Quantity: int64(ord.Qty),
				})
			}
		}
	}
}

// isShutdownErr returns true if err is the result of ctx cancellation (graceful
// shutdown) and should not be counted as a real failure.
func isShutdownErr(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
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
