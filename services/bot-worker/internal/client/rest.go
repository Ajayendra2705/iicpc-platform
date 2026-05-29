package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	BaseURL   string
	Timeout   time.Duration     // 0 → 5s
	Transport http.RoundTripper // 0 → http.DefaultTransport (override for tuned pooling / mocks)
}

type placeReq struct {
	Side  string  `json:"side"`
	Kind  string  `json:"kind,omitempty"` // "limit" (default) or "market"
	Price float64 `json:"price,omitempty"`
	Qty   int     `json:"qty"`
}

// fill mirrors one match from the orderbook contract response. Only Qty is
// needed to total the contestant-reported fill for an order.
type fill struct {
	Qty int64 `json:"qty"`
}

type placeResp struct {
	ID    string `json:"id"`
	Fills []fill `json:"fills"`
}

type REST struct {
	baseURL string
	hc      *http.Client
}

func New(cfg Config) *REST {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &REST{
		baseURL: cfg.BaseURL,
		hc:      &http.Client{Timeout: cfg.Timeout, Transport: cfg.Transport},
	}
}

// PlaceOrder sends a new limit order to the target orderbook.
// Returns the placement outcome (assigned ID, reported fill, RTT) and any error.
func (c *REST) PlaceOrder(ctx context.Context, side string, price float64, qty int) (PlaceResult, error) {
	return c.place(ctx, placeReq{Side: side, Kind: "limit", Price: price, Qty: qty})
}

// PlaceMarketOrder sends a market order (no price, IOC). Returns the placement
// outcome (assigned ID, reported fill, RTT) and any error.
func (c *REST) PlaceMarketOrder(ctx context.Context, side string, qty int) (PlaceResult, error) {
	return c.place(ctx, placeReq{Side: side, Kind: "market", Qty: qty})
}

func (c *REST) place(ctx context.Context, pr placeReq) (PlaceResult, error) {
	body, err := json.Marshal(pr)
	if err != nil {
		return PlaceResult{}, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/order", bytes.NewReader(body))
	if err != nil {
		return PlaceResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.hc.Do(req)
	latNs := time.Since(start).Nanoseconds()
	if err != nil {
		return PlaceResult{LatencyNs: latNs}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return PlaceResult{LatencyNs: latNs}, fmt.Errorf("place order: status %d", resp.StatusCode)
	}
	var resBody placeResp
	if err := json.NewDecoder(resp.Body).Decode(&resBody); err != nil {
		return PlaceResult{LatencyNs: latNs}, fmt.Errorf("decode response: %w", err)
	}
	// REST contract always echoes the (possibly empty) fills array, so the
	// reported fill is authoritative even when zero (order rested, no cross).
	var filled int64
	for _, f := range resBody.Fills {
		filled += f.Qty
	}
	return PlaceResult{ID: resBody.ID, Filled: filled, FillKnown: true, LatencyNs: latNs}, nil
}

func (c *REST) Close() error { return nil }

// CancelOrder cancels an existing order by ID.
// Returns round-trip latency in nanoseconds and any error.
func (c *REST) CancelOrder(ctx context.Context, id string) (latNs int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/order/"+id, nil)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := c.hc.Do(req)
	latNs = time.Since(start).Nanoseconds()
	if err != nil {
		return latNs, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return latNs, fmt.Errorf("cancel order: status %d", resp.StatusCode)
	}
	return latNs, nil
}
