package ingest

// AggSnapshot mirrors the JSON shape returned by aggregator's GET /metrics.
// Decoupled from the upstream type so leaderboard-svc doesn't depend on the
// aggregator module.
type AggSnapshot struct {
	ContestantID string  `json:"contestant_id"`
	Count        int64   `json:"count"`
	Rejected     int64   `json:"rejected"`
	Timeouts     int64   `json:"timeouts"`
	TPS          float64 `json:"tps"`
	P50Ns        int64   `json:"p50_ns"`
	P90Ns        int64   `json:"p90_ns"`
	P99Ns        int64   `json:"p99_ns"`
}

// ValReport mirrors validator's GET /validate row.
type ValReport struct {
	ContestantID string  `json:"contestant_id"`
	TotalChecked int64   `json:"total_checked"`
	Mismatches   int64   `json:"mismatches"`
	Correctness  float64 `json:"correctness"`
}
