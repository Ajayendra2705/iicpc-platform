package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type wsReq struct {
	Action string  `json:"action"`
	Side   string  `json:"side,omitempty"`
	Kind   string  `json:"kind,omitempty"` // "limit" (default) or "market"
	Price  float64 `json:"price,omitempty"`
	Qty    int     `json:"qty,omitempty"`
	ID     string  `json:"id,omitempty"`
}

type wsResp struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	// Filled is the optional reported fill quantity. A pointer so we can tell
	// "reported zero fill" (authoritative) from "field absent" (unknown).
	Filled *int64 `json:"filled,omitempty"`
}

// WSClient sends orders over a WebSocket connection.
// One connection per goroutine — not safe for concurrent use.
type WSClient struct {
	conn *websocket.Conn
}

// DialWS connects to the given URL (http:// and ws:// both accepted) and
// returns a ready-to-use WSClient.
func DialWS(ctx context.Context, rawURL string) (*WSClient, error) {
	wsURL := strings.Replace(rawURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ws dial %s: %w", wsURL, err)
	}
	return &WSClient{conn: conn}, nil
}

func (c *WSClient) PlaceOrder(ctx context.Context, side string, price float64, qty int) (PlaceResult, error) {
	return c.place(ctx, wsReq{Action: "place", Side: side, Kind: "limit", Price: price, Qty: qty})
}

func (c *WSClient) PlaceMarketOrder(ctx context.Context, side string, qty int) (PlaceResult, error) {
	return c.place(ctx, wsReq{Action: "place", Side: side, Kind: "market", Qty: qty})
}

func (c *WSClient) place(ctx context.Context, req wsReq) (PlaceResult, error) {
	resp, latNs, err := c.roundTrip(ctx, req)
	if err != nil {
		return PlaceResult{LatencyNs: latNs}, err
	}
	res := PlaceResult{ID: resp.ID, LatencyNs: latNs}
	if resp.Filled != nil {
		res.Filled = *resp.Filled
		res.FillKnown = true
	}
	return res, nil
}

func (c *WSClient) CancelOrder(ctx context.Context, id string) (int64, error) {
	_, latNs, err := c.roundTrip(ctx, wsReq{Action: "cancel", ID: id})
	return latNs, err
}

func (c *WSClient) Close() error {
	_ = c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	return c.conn.Close()
}

func (c *WSClient) roundTrip(ctx context.Context, req wsReq) (resp wsResp, latNs int64, err error) {
	msg, err := json.Marshal(req)
	if err != nil {
		return wsResp{}, 0, fmt.Errorf("ws marshal: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	_ = c.conn.SetWriteDeadline(deadline)
	_ = c.conn.SetReadDeadline(deadline)

	start := time.Now()
	if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		return wsResp{}, 0, fmt.Errorf("ws write: %w", err)
	}
	_, raw, err := c.conn.ReadMessage()
	latNs = time.Since(start).Nanoseconds()
	if err != nil {
		return wsResp{}, latNs, fmt.Errorf("ws read: %w", err)
	}

	if err := json.Unmarshal(raw, &resp); err != nil {
		return wsResp{}, latNs, fmt.Errorf("ws decode: %w", err)
	}
	if resp.Error != "" {
		return wsResp{}, latNs, fmt.Errorf("ws server error: %s", resp.Error)
	}
	return resp, latNs, nil
}
