package windowing

import (
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

// MergedSnapshot is the result of merging N windowed Snapshots correctly:
// the underlying histograms are summed bucket-by-bucket so the merged
// percentiles are *exact*, not averages of per-window percentiles.
//
// Averaging percentiles is statistically wrong — a contestant could have
// P99=1ms in window A and P99=100ms in window B; the merged P99 is whatever
// the merged histogram says, which is typically much closer to 100ms than
// to 50.5ms. Without histogram-level merge, the leaderboard would systematically
// understate tail latency.
type MergedSnapshot struct {
	ContestantID  string        `json:"contestant_id"`
	WindowCount   int           `json:"window_count"`
	Duration      time.Duration `json:"duration_ns"`
	TotalCount    int64         `json:"count"`
	TotalRejected int64         `json:"rejected"`
	TotalTimeouts int64         `json:"timeouts"`
	AverageTPS    float64       `json:"avg_tps"`
	P50Ns         int64         `json:"p50_ns"`
	P90Ns         int64         `json:"p90_ns"`
	P99Ns         int64         `json:"p99_ns"`
	P999Ns        int64         `json:"p999_ns"`
}

// MergeHistograms folds N hdrhistograms into one and returns the merged
// percentile cuts. Caller passes the histograms (defensive copies are
// taken internally — input is not mutated).
func MergeHistograms(hs []*hdrhistogram.Histogram) *hdrhistogram.Histogram {
	if len(hs) == 0 {
		return nil
	}
	merged := hdrhistogram.New(defaultMinLatNs, defaultMaxLatNs, defaultSigDigits)
	for _, h := range hs {
		if h == nil {
			continue
		}
		// hdrhistogram-go Merge mutates the receiver but reads other immutably.
		_ = merged.Merge(h)
	}
	return merged
}
