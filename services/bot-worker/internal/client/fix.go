package client

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quickfixgo/quickfix"
)

// FIX 4.4 tag constants used by the bot.
const (
	tagMsgType      quickfix.Tag = 35
	tagClOrdID      quickfix.Tag = 11
	tagOrigClOrdID  quickfix.Tag = 41
	tagSymbol       quickfix.Tag = 55
	tagSide         quickfix.Tag = 54
	tagOrdType      quickfix.Tag = 40
	tagPrice        quickfix.Tag = 44
	tagOrderQty     quickfix.Tag = 38
	tagTransactTime quickfix.Tag = 60
	tagOrderID      quickfix.Tag = 37
)

type fixResult struct {
	orderID string
}

// fixApp implements quickfix.Application and routes inbound ExecutionReports
// to the goroutines waiting in PlaceOrder.
type fixApp struct {
	mu        sync.Mutex
	sessionID quickfix.SessionID
	ready     chan struct{}
	readyOnce sync.Once
	pending   map[string]chan fixResult
	log       *slog.Logger
}

func newFixApp(log *slog.Logger) *fixApp {
	return &fixApp{
		ready:   make(chan struct{}),
		pending: make(map[string]chan fixResult),
		log:     log,
	}
}

func (a *fixApp) OnCreate(_ quickfix.SessionID)                         {}
func (a *fixApp) ToAdmin(_ *quickfix.Message, _ quickfix.SessionID)     {}
func (a *fixApp) ToApp(_ *quickfix.Message, _ quickfix.SessionID) error { return nil }
func (a *fixApp) FromAdmin(_ *quickfix.Message, _ quickfix.SessionID) quickfix.MessageRejectError {
	return nil
}

func (a *fixApp) OnLogon(sessionID quickfix.SessionID) {
	a.mu.Lock()
	a.sessionID = sessionID
	a.mu.Unlock()
	a.readyOnce.Do(func() { close(a.ready) })
	a.log.Info("FIX session logon", "session", sessionID.String())
}

func (a *fixApp) OnLogout(sessionID quickfix.SessionID) {
	a.log.Warn("FIX session logout", "session", sessionID.String())
}

func (a *fixApp) FromApp(msg *quickfix.Message, _ quickfix.SessionID) quickfix.MessageRejectError {
	msgType, err := msg.Header.GetString(tagMsgType)
	if err != nil || msgType != "8" { // 8 = ExecutionReport
		return nil
	}
	clOrdID, err := msg.Body.GetString(tagClOrdID)
	if err != nil {
		return nil
	}
	orderID, _ := msg.Body.GetString(tagOrderID)

	a.mu.Lock()
	ch, ok := a.pending[clOrdID]
	if ok {
		delete(a.pending, clOrdID)
	}
	a.mu.Unlock()

	if ok {
		ch <- fixResult{orderID: orderID}
	}
	return nil
}

// FIXConfig holds parameters for a FIX 4.4 initiator session.
type FIXConfig struct {
	TargetHost   string
	TargetPort   int
	SenderCompID string // default: BOTWORKER
	TargetCompID string // default: EXCHANGE
	Symbol       string // instrument, default: BTCUSD
}

func (c *FIXConfig) applyDefaults() {
	if c.SenderCompID == "" {
		c.SenderCompID = "BOTWORKER"
	}
	if c.TargetCompID == "" {
		c.TargetCompID = "EXCHANGE"
	}
	if c.Symbol == "" {
		c.Symbol = "BTCUSD"
	}
	if c.TargetPort == 0 {
		c.TargetPort = 5001
	}
}

// FIXClient sends FIX 4.4 NewOrderSingle messages to a contestant's exchange.
// Shared across goroutines; one FIX session, concurrent PlaceOrder calls safe.
type FIXClient struct {
	app       *fixApp
	initiator *quickfix.Initiator
	symbol    string
	seq       atomic.Uint64
}

// NewFIX creates a FIXClient. Call Connect before placing orders.
func NewFIX(cfg FIXConfig, log *slog.Logger) (*FIXClient, error) {
	cfg.applyDefaults()

	settingsTxt := fmt.Sprintf(`
[DEFAULT]
ConnectionType=initiator
HeartBtInt=30
ResetOnLogon=Y
SenderCompID=%s
TargetCompID=%s

[SESSION]
BeginString=FIX.4.4
SocketConnectHost=%s
SocketConnectPort=%d
`, cfg.SenderCompID, cfg.TargetCompID, cfg.TargetHost, cfg.TargetPort)

	settings, err := quickfix.ParseSettings(strings.NewReader(settingsTxt))
	if err != nil {
		return nil, fmt.Errorf("fix settings: %w", err)
	}

	app := newFixApp(log)
	initiator, err := quickfix.NewInitiator(app, quickfix.NewMemoryStoreFactory(), settings, quickfix.NewNullLogFactory())
	if err != nil {
		return nil, fmt.Errorf("fix initiator: %w", err)
	}

	return &FIXClient{app: app, initiator: initiator, symbol: cfg.Symbol}, nil
}

// Connect starts the FIX initiator and blocks until logon completes or ctx is cancelled.
func (c *FIXClient) Connect(ctx context.Context) error {
	if err := c.initiator.Start(); err != nil {
		return fmt.Errorf("fix start: %w", err)
	}
	select {
	case <-c.app.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PlaceOrder sends a FIX 4.4 NewOrderSingle and waits for an ExecutionReport.
// Returns the exchange OrderID, RTT in nanoseconds, and any error.
func (c *FIXClient) PlaceOrder(ctx context.Context, side string, price float64, qty int) (string, int64, error) {
	clOrdID := fmt.Sprintf("bot-%d", c.seq.Add(1))

	fixSide := "1" // buy
	if side == "sell" {
		fixSide = "2"
	}

	msg := quickfix.NewMessage()
	msg.Header.SetField(tagMsgType, quickfix.FIXString("D"))
	msg.Body.SetField(tagClOrdID, quickfix.FIXString(clOrdID))
	msg.Body.SetField(tagSymbol, quickfix.FIXString(c.symbol))
	msg.Body.SetField(tagSide, quickfix.FIXString(fixSide))
	msg.Body.SetField(tagOrdType, quickfix.FIXString("2")) // limit
	msg.Body.SetField(tagPrice, quickfix.FIXString(fmt.Sprintf("%.2f", price)))
	msg.Body.SetField(tagOrderQty, quickfix.FIXString(fmt.Sprintf("%d", qty)))
	msg.Body.SetField(tagTransactTime, quickfix.FIXString(time.Now().UTC().Format("20060102-15:04:05.000")))

	ch := make(chan fixResult, 1)
	c.app.mu.Lock()
	c.app.pending[clOrdID] = ch
	sessionID := c.app.sessionID
	c.app.mu.Unlock()

	start := time.Now()
	if err := quickfix.SendToTarget(msg, sessionID); err != nil {
		c.app.mu.Lock()
		delete(c.app.pending, clOrdID)
		c.app.mu.Unlock()
		return "", 0, fmt.Errorf("fix send: %w", err)
	}

	select {
	case res := <-ch:
		return res.orderID, time.Since(start).Nanoseconds(), nil
	case <-ctx.Done():
		c.app.mu.Lock()
		delete(c.app.pending, clOrdID)
		c.app.mu.Unlock()
		return "", 0, ctx.Err()
	case <-time.After(5 * time.Second):
		c.app.mu.Lock()
		delete(c.app.pending, clOrdID)
		c.app.mu.Unlock()
		return "", 0, fmt.Errorf("fix: ExecutionReport timeout for %s", clOrdID)
	}
}

// CancelOrder sends a FIX 4.4 OrderCancelRequest (35=F) and waits for
// an ExecutionReport acknowledging the cancel.
func (c *FIXClient) CancelOrder(ctx context.Context, origOrderID string) (int64, error) {
	clOrdID := fmt.Sprintf("cxl-%d", c.seq.Add(1))

	msg := quickfix.NewMessage()
	msg.Header.SetField(tagMsgType, quickfix.FIXString("F"))
	msg.Body.SetField(tagClOrdID, quickfix.FIXString(clOrdID))
	msg.Body.SetField(tagOrigClOrdID, quickfix.FIXString(origOrderID))
	msg.Body.SetField(tagSymbol, quickfix.FIXString(c.symbol))
	msg.Body.SetField(tagSide, quickfix.FIXString("1"))
	msg.Body.SetField(tagOrderQty, quickfix.FIXString("1"))
	msg.Body.SetField(tagTransactTime, quickfix.FIXString(time.Now().UTC().Format("20060102-15:04:05.000")))

	ch := make(chan fixResult, 1)
	c.app.mu.Lock()
	c.app.pending[clOrdID] = ch
	sessionID := c.app.sessionID
	c.app.mu.Unlock()

	start := time.Now()
	if err := quickfix.SendToTarget(msg, sessionID); err != nil {
		c.app.mu.Lock()
		delete(c.app.pending, clOrdID)
		c.app.mu.Unlock()
		return 0, fmt.Errorf("fix cancel send: %w", err)
	}

	select {
	case <-ch:
		return time.Since(start).Nanoseconds(), nil
	case <-ctx.Done():
		c.app.mu.Lock()
		delete(c.app.pending, clOrdID)
		c.app.mu.Unlock()
		return 0, ctx.Err()
	case <-time.After(5 * time.Second):
		c.app.mu.Lock()
		delete(c.app.pending, clOrdID)
		c.app.mu.Unlock()
		return 0, fmt.Errorf("fix: CancelAck timeout for %s", clOrdID)
	}
}

func (c *FIXClient) Close() error {
	c.initiator.Stop()
	return nil
}
