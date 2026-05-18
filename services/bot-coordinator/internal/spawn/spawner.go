package spawn

import "context"

// BenchmarkSpec describes a load benchmark run.
type BenchmarkSpec struct {
	SubmissionID    string  `json:"submission_id"`
	TargetURL       string  `json:"target_url"`
	NumWorkers      int     `json:"num_workers"`
	OrdersPerSecond float64 `json:"orders_per_second"`
	Protocol        string  `json:"protocol"`     // rest | ws | fix
	ArrivalMode     string  `json:"arrival_mode"` // uniform | poisson
	DurationSeconds int     `json:"duration_seconds"`
	// Profiles is the list of trader archetypes the bot fleet should
	// simulate (PS §"Distributed Load Generator": "diverse market
	// participants"). The coordinator spawns one indexed Job whose pods
	// are partitioned across these profiles by pod index. Empty/nil →
	// single "noise" profile (back-compat).
	Profiles []string `json:"profiles,omitempty"`
}

// JobStatus reports the live state of a benchmark's K8s Job.
type JobStatus struct {
	Active    int32  `json:"active"`
	Succeeded int32  `json:"succeeded"`
	Failed    int32  `json:"failed"`
	Phase     string `json:"phase"` // running | succeeded | failed
}

// JobSpawner manages the lifecycle of bot-worker K8s Jobs.
type JobSpawner interface {
	Start(ctx context.Context, benchmarkID string, spec BenchmarkSpec) error
	Stop(ctx context.Context, benchmarkID string) error
	Status(ctx context.Context, benchmarkID string) (JobStatus, error)
}
