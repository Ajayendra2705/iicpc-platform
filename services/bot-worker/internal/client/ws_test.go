package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/client"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// echoOrderServer upgrades HTTP to WS, reads a wsReq, responds with a wsResp.
func echoOrderServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("ws upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req map[string]any
			_ = json.Unmarshal(raw, &req)
			time.Sleep(time.Millisecond) // guarantee measurable latency on Windows
			var resp []byte
			if req["action"] == "cancel" {
				resp, _ = json.Marshal(map[string]string{"id": req["id"].(string), "status": "ok"})
			} else {
				resp, _ = json.Marshal(map[string]string{"id": "ws-ord-1"})
			}
			_ = conn.WriteMessage(websocket.TextMessage, resp)
		}
	}))
}

func wsURL(srv *httptest.Server) string {
	return strings.Replace(srv.URL, "http://", "ws://", 1) + "/ws"
}

func TestWSPlaceOrder(t *testing.T) {
	srv := echoOrderServer(t)
	defer srv.Close()

	c, err := client.DialWS(context.Background(), wsURL(srv))
	if err != nil {
		t.Fatalf("DialWS: %v", err)
	}
	defer c.Close()

	res, err := c.PlaceOrder(context.Background(), "buy", 100.5, 5)
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if res.ID != "ws-ord-1" {
		t.Errorf("id: got %q want ws-ord-1", res.ID)
	}
	if res.LatencyNs <= 0 {
		t.Error("latNs must be positive")
	}
	// Echo server sends no "filled" field → fill is not authoritative.
	if res.FillKnown {
		t.Error("fill should be unknown when response omits the filled field")
	}
}

func TestWSCancelOrder(t *testing.T) {
	srv := echoOrderServer(t)
	defer srv.Close()

	c, err := client.DialWS(context.Background(), wsURL(srv))
	if err != nil {
		t.Fatalf("DialWS: %v", err)
	}
	defer c.Close()

	latNs, err := c.CancelOrder(context.Background(), "ord-42")
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if latNs <= 0 {
		t.Error("latNs must be positive")
	}
}

func TestWSServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		resp, _ := json.Marshal(map[string]string{"id": "", "error": "rejected"})
		_ = conn.WriteMessage(websocket.TextMessage, resp)
	}))
	defer srv.Close()

	c, err := client.DialWS(context.Background(), wsURL(srv))
	if err != nil {
		t.Fatalf("DialWS: %v", err)
	}
	defer c.Close()

	_, err = c.PlaceOrder(context.Background(), "buy", 100, 1)
	if err == nil {
		t.Fatal("expected error from server error response")
	}
}

func TestWSDialInvalidURL(t *testing.T) {
	_, err := client.DialWS(context.Background(), "ws://127.0.0.1:1") // nothing listening
	if err == nil {
		t.Fatal("expected error dialing bad URL")
	}
}
