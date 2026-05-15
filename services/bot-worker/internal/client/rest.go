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
	BaseURL string
	Timeout time.Duration // 0 → 5s
}

type placeReq struct {
	Side  string  `json:"side"`
	Price float64 `json:"price"`
	Qty   int     `json:"qty"`
}

type placeResp struct {
	ID string `json:"id"`
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
		hc:      &http.Client{Timeout: cfg.Timeout},
	}
}

// PlaceOrder sends a new order to the target orderbook.
// Returns the assigned order ID, round-trip latency in nanoseconds, and any error.
func (c *REST) PlaceOrder(ctx context.Context, side string, price float64, qty int) (id string, latNs int64, err error) {
	body, err := json.Marshal(placeReq{Side: side, Price: price, Qty: qty})
	if err != nil {
		return "", 0, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/order", bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.hc.Do(req)
	latNs = time.Since(start).Nanoseconds()
	if err != nil {
		return "", latNs, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", latNs, fmt.Errorf("place order: status %d", resp.StatusCode)
	}
	var pr placeResp
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", latNs, fmt.Errorf("decode response: %w", err)
	}
	return pr.ID, latNs, nil
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
