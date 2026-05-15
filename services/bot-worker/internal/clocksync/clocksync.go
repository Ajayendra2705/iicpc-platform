package clocksync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type timeResp struct {
	NowNs int64 `json:"now_ns"`
}

// EstimateOffset probes targetURL+"/time" and returns the estimated clock offset
// in nanoseconds using a single NTP-style round trip.
// Positive offset means local clock is ahead of the server.
func EstimateOffset(ctx context.Context, hc *http.Client, targetURL string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL+"/time", nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}

	t1 := time.Now().UnixNano()
	resp, err := hc.Do(req)
	if err != nil {
		return 0, fmt.Errorf("probe: %w", err)
	}
	defer resp.Body.Close()
	t3 := time.Now().UnixNano()

	var body timeResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}

	midpoint := (t1 + t3) / 2
	return body.NowNs - midpoint, nil
}
