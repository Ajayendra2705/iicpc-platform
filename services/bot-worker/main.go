package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
	marketRatio     float64 // fraction of orders sent as Market (IOC, no price); default 0.10
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
	botProfile      string // PS-mandated diverse trader archetype: market_maker | aggressive_taker | retail | noise
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
		marketRatio:     envFloat("MARKET_RATIO", 0.10),
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
		botProfile:      resolveProfile(),
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfig()

	rec := stats.New()
	// If BOT_PROFILE is set, use the preset Config that represents the
	// PS-mandated "diverse market participant" archetype. Otherwise fall back
	// to the per-env-var settings (mid/sigma/cancel/market) for finer tuning.
	var genCfg gen.Config
	if cfg.botProfile != "" {
		c, err := gen.PresetConfig(gen.Profile(cfg.botProfile), cfg.midPrice, cfg.priceSigma)
		if err != nil {
			logger.Error("bot profile invalid", "err", err)
			os.Exit(1)
		}
		genCfg = c
		logger.Info("bot profile applied", "profile", cfg.botProfile,
			"cancel_ratio", genCfg.CancelRatio, "market_ratio", genCfg.MarketRatio,
			"price_sigma", genCfg.PriceSigma)
	} else {
		genCfg = gen.Config{
			MidPrice:    cfg.midPrice,
			PriceSigma:  cfg.priceSigma,
			CancelRatio: cfg.cancelRatio,
			MarketRatio: cfg.marketRatio,
		}
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
						Result:   resultForErr(err),
						SentTsNs: start.UnixNano(),
					})
				}
			}

			// place a new order on every arrival
			ord := g.Next()
			start := time.Now()
			var res client.PlaceResult
			var err error
			teleType := telemetryv1.OrderType_ORDER_TYPE_LIMIT
			if ord.Kind == gen.Market {
				teleType = telemetryv1.OrderType_ORDER_TYPE_MARKET
				res, err = cli.PlaceMarketOrder(ctx, string(ord.Side), ord.Qty)
			} else {
				res, err = cli.PlaceOrder(ctx, string(ord.Side), ord.Price, ord.Qty)
			}
			teleSide := sideEnum(string(ord.Side))
			switch {
			case err == nil:
				rec.RecordLatency(res.LatencyNs)
				// Market orders are IOC: they don't rest on the book, so don't
				// track for later cancellation. Only limit orders go to liveIDs.
				if ord.Kind != gen.Market {
					liveIDs = append(liveIDs, res.ID)
				}
				// Classify the fill so the validator can score accuracy. When the
				// transport didn't parse fills (FillKnown=false), leave the result
				// UNSPECIFIED so the validator replays for book state but does not
				// score a fill it cannot trust.
				result := telemetryv1.OrderResult_ORDER_RESULT_UNSPECIFIED
				var filled int64
				if res.FillKnown {
					filled = res.Filled
					switch {
					case filled >= int64(ord.Qty):
						result = telemetryv1.OrderResult_ORDER_RESULT_FILLED
					case filled > 0:
						result = telemetryv1.OrderResult_ORDER_RESULT_PARTIAL
					default:
						result = telemetryv1.OrderResult_ORDER_RESULT_ACK_ONLY
					}
				}
				tele.Emit(&telemetryv1.OrderEvent{
					ContestantId: contestantID, BotId: botID, OrderId: res.ID,
					Type:     teleType,
					Side:     teleSide,
					Result:   result,
					SentTsNs: start.UnixNano(), AckTsNs: start.Add(time.Duration(res.LatencyNs)).UnixNano(),
					LatencyNs: res.LatencyNs, Price: ord.Price, Quantity: int64(ord.Qty),
					FilledQuantity: filled,
				})
			case isShutdownErr(ctx, err):
				return
			default:
				rec.RecordError()
				log.Warn("place failed", "kind", ord.Kind, "err", err)
				tele.Emit(&telemetryv1.OrderEvent{
					ContestantId: contestantID, BotId: botID,
					Type:     teleType,
					Side:     teleSide,
					Result:   resultForErr(err),
					SentTsNs: start.UnixNano(),
					Price:    ord.Price, Quantity: int64(ord.Qty),
				})
			}
		}
	}
}

// isShutdownErr returns true only when err is the result of *parent* context
// cancellation (graceful shutdown). A per-request client timeout
// (http.Client.Timeout) must NOT be treated as shutdown — it is a real
// contestant timeout and is scored as ORDER_RESULT_TIMEOUT via resultForErr.
func isShutdownErr(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled)
}

// resultForErr classifies a non-shutdown request failure. A deadline/timeout
// (the contestant didn't acknowledge in time) is scored as a TIMEOUT and feeds
// the stability penalty; anything else is an outright REJECTED.
func resultForErr(err error) telemetryv1.OrderResult {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return telemetryv1.OrderResult_ORDER_RESULT_TIMEOUT
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return telemetryv1.OrderResult_ORDER_RESULT_TIMEOUT
	}
	return telemetryv1.OrderResult_ORDER_RESULT_REJECTED
}

// sideEnum maps the generator's buy/sell string to the telemetry OrderSide so
// the validator can replay price-time priority without guessing from the id.
func sideEnum(side string) telemetryv1.OrderSide {
	if strings.EqualFold(side, "sell") {
		return telemetryv1.OrderSide_ORDER_SIDE_SELL
	}
	return telemetryv1.OrderSide_ORDER_SIDE_BUY
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

// resolveProfile picks the trader-archetype profile for this bot pod.
// Priority:
//  1. BOT_PROFILE explicit (e.g., set by operator or test) — used as-is.
//  2. BOT_PROFILES (comma list) + JOB_COMPLETION_INDEX from an Indexed Job —
//     pick profiles[index % len(profiles)] so the fleet rotates across the
//     PS-mandated diverse market participants.
//  3. Empty → bot-worker falls through to per-env Config (legacy behaviour).
func resolveProfile() string {
	if p := os.Getenv("BOT_PROFILE"); p != "" {
		return p
	}
	list := os.Getenv("BOT_PROFILES")
	idxStr := os.Getenv("JOB_COMPLETION_INDEX")
	if list == "" || idxStr == "" {
		return ""
	}
	parts := strings.Split(list, ",")
	if len(parts) == 0 {
		return ""
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		return parts[0]
	}
	return strings.TrimSpace(parts[idx%len(parts)])
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
