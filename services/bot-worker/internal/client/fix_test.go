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

	_ = app.FromApp(msg, quickfix.SessionID{})

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

func TestFixAppFromAppParsesCumQty(t *testing.T) {
	app := newFixApp(testLogger())
	ch := make(chan fixResult, 1)
	app.mu.Lock()
	app.pending["ord-010"] = ch
	app.mu.Unlock()

	msg := quickfix.NewMessage()
	msg.Header.SetField(tagMsgType, quickfix.FIXString("8"))
	msg.Body.SetField(tagClOrdID, quickfix.FIXString("ord-010"))
	msg.Body.SetField(tagOrderID, quickfix.FIXString("exch-1"))
	msg.Body.SetField(tagCumQty, quickfix.FIXString("7"))

	_ = app.FromApp(msg, quickfix.SessionID{})

	res := <-ch
	if !res.fillKnown || res.cumQty != 7 {
		t.Fatalf("cumQty: got known=%v qty=%d want known=true qty=7", res.fillKnown, res.cumQty)
	}
}

func TestFixAppFromAppNoCumQtyIsUnknown(t *testing.T) {
	app := newFixApp(testLogger())
	ch := make(chan fixResult, 1)
	app.mu.Lock()
	app.pending["ord-011"] = ch
	app.mu.Unlock()

	msg := quickfix.NewMessage()
	msg.Header.SetField(tagMsgType, quickfix.FIXString("8"))
	msg.Body.SetField(tagClOrdID, quickfix.FIXString("ord-011"))
	msg.Body.SetField(tagOrderID, quickfix.FIXString("exch-2"))
	// no CumQty → fill not authoritative

	_ = app.FromApp(msg, quickfix.SessionID{})

	res := <-ch
	if res.fillKnown {
		t.Fatal("fill must be unknown when ExecutionReport omits CumQty")
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
	_ = app.FromApp(msg, quickfix.SessionID{})

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
	_ = app.FromApp(msg, quickfix.SessionID{})
}

// Cancel's ExecReport (35=8) carries the cancel clOrdID in tag 11 — same routing.
func TestFixAppFromAppRoutesCancelAck(t *testing.T) {
	app := newFixApp(testLogger())
	ch := make(chan fixResult, 1)
	app.mu.Lock()
	app.pending["cxl-001"] = ch
	app.mu.Unlock()

	msg := quickfix.NewMessage()
	msg.Header.SetField(tagMsgType, quickfix.FIXString("8"))
	msg.Body.SetField(tagClOrdID, quickfix.FIXString("cxl-001"))
	msg.Body.SetField(tagOrderID, quickfix.FIXString("exch-999"))
	_ = app.FromApp(msg, quickfix.SessionID{})

	select {
	case res := <-ch:
		if res.orderID != "exch-999" {
			t.Errorf("cancel ack orderID: got %q want exch-999", res.orderID)
		}
	default:
		t.Fatal("cancel ack not routed")
	}
}

func TestOrigClOrdIDTagValue(t *testing.T) {
	if tagOrigClOrdID != 41 {
		t.Errorf("tagOrigClOrdID: got %d want 41", tagOrigClOrdID)
	}
}

// TestBuildCancelReferencesOrigClOrdIDAndSide locks in the FIX 4.4 cancel fix:
// the OrderCancelRequest references the ORIGINAL order's ClOrdID via tag 41 (not
// the exchange OrderID) and carries its Side via tag 54.
func TestBuildCancelReferencesOrigClOrdIDAndSide(t *testing.T) {
	msg := buildCancel("cxl-7", "bot-3", "2", "ETHUSD")

	if mt, _ := msg.Header.GetString(tagMsgType); mt != "F" {
		t.Errorf("MsgType(35): got %q want F", mt)
	}
	if v, _ := msg.Body.GetString(tagClOrdID); v != "cxl-7" {
		t.Errorf("ClOrdID(11): got %q want cxl-7", v)
	}
	if v, _ := msg.Body.GetString(tagOrigClOrdID); v != "bot-3" {
		t.Errorf("OrigClOrdID(41): got %q want bot-3 (original order's ClOrdID, not exchange OrderID)", v)
	}
	if v, _ := msg.Body.GetString(tagSide); v != "2" {
		t.Errorf("Side(54): got %q want 2 (original order's side)", v)
	}
	if v, _ := msg.Body.GetString(tagSymbol); v != "ETHUSD" {
		t.Errorf("Symbol(55): got %q want ETHUSD", v)
	}
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
