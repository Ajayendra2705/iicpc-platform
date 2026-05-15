package client

import (
	"log/slog"
	"os"
	"testing"

	"github.com/quickfixgo/quickfix"
)

// Tests for fixApp — the Application callback layer.
// Full FIX session tests require a live acceptor; only the Application
// logic is tested here in-process.

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestFixAppOnLogonClosesReady(t *testing.T) {
	app := newFixApp(testLogger())
	sid := quickfix.SessionID{BeginString: "FIX.4.4", SenderCompID: "BOT", TargetCompID: "EX"}

	app.OnLogon(sid)

	select {
	case <-app.ready:
	default:
		t.Fatal("ready channel not closed after OnLogon")
	}

	// second OnLogon must not panic (readyOnce guard)
	app.OnLogon(sid)
}

func TestFixAppOnLogonSetsSessionID(t *testing.T) {
	app := newFixApp(testLogger())
	sid := quickfix.SessionID{BeginString: "FIX.4.4", SenderCompID: "BOT", TargetCompID: "EX"}
	app.OnLogon(sid)

	app.mu.Lock()
	got := app.sessionID
	app.mu.Unlock()

	if got != sid {
		t.Fatalf("sessionID: got %v want %v", got, sid)
	}
}

func TestFixAppFromAppRoutesExecutionReport(t *testing.T) {
	app := newFixApp(testLogger())
	ch := make(chan fixResult, 1)

	app.mu.Lock()
	app.pending["ord-001"] = ch
	app.mu.Unlock()

	// Build a minimal ExecutionReport message.
	msg := quickfix.NewMessage()
	msg.Header.SetField(tagMsgType, quickfix.FIXString("8"))
	msg.Body.SetField(tagClOrdID, quickfix.FIXString("ord-001"))
	msg.Body.SetField(tagOrderID, quickfix.FIXString("exch-999"))

	app.FromApp(msg, quickfix.SessionID{})

	select {
	case res := <-ch:
		if res.orderID != "exch-999" {
			t.Errorf("orderID: got %q want exch-999", res.orderID)
		}
	default:
		t.Fatal("pending channel not signalled")
	}

	// pending entry must be removed
	app.mu.Lock()
	_, still := app.pending["ord-001"]
	app.mu.Unlock()
	if still {
		t.Error("pending entry not deleted after routing")
	}
}

func TestFixAppFromAppIgnoresNonExecutionReport(t *testing.T) {
	app := newFixApp(testLogger())
	ch := make(chan fixResult, 1)
	app.mu.Lock()
	app.pending["ord-002"] = ch
	app.mu.Unlock()

	// Send a Heartbeat (35=0) — should be ignored.
	msg := quickfix.NewMessage()
	msg.Header.SetField(tagMsgType, quickfix.FIXString("0"))
	app.FromApp(msg, quickfix.SessionID{})

	select {
	case <-ch:
		t.Fatal("channel should not be signalled for non-ExecutionReport")
	default:
	}
}

func TestFixAppFromAppIgnoresMissingClOrdID(t *testing.T) {
	app := newFixApp(testLogger())

	msg := quickfix.NewMessage()
	msg.Header.SetField(tagMsgType, quickfix.FIXString("8"))
	// no ClOrdID set

	// must not panic
	app.FromApp(msg, quickfix.SessionID{})
}

func TestFIXClientImplementsOrderClient(t *testing.T) {
	// Compile-time check that *FIXClient satisfies OrderClient.
	var _ OrderClient = (*FIXClient)(nil)
}

func TestWSClientImplementsOrderClient(t *testing.T) {
	var _ OrderClient = (*WSClient)(nil)
}

func TestRESTClientImplementsOrderClient(t *testing.T) {
	var _ OrderClient = (*REST)(nil)
}

// Verify FIXConfig defaults are applied correctly.
func TestFIXConfigDefaults(t *testing.T) {
	cfg := FIXConfig{}
	cfg.applyDefaults()
	if cfg.SenderCompID != "BOTWORKER" {
		t.Errorf("SenderCompID default: got %q", cfg.SenderCompID)
	}
	if cfg.Symbol != "BTCUSD" {
		t.Errorf("Symbol default: got %q", cfg.Symbol)
	}
	if cfg.TargetPort != 5001 {
		t.Errorf("TargetPort default: got %d", cfg.TargetPort)
	}
}

