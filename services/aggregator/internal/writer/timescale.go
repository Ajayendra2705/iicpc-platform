package writer

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

// Timescale writes Snapshots to a TimescaleDB hypertable using pgx.CopyFrom
// for high-throughput inserts (much faster than batched INSERTs at >100 rows/s).
type Timescale struct {
	pool *pgxpool.Pool
}

func NewTimescale(pool *pgxpool.Pool) *Timescale {
	return &Timescale{pool: pool}
}

// NewTimescaleFromDSN dials pool. Caller still owns the pool unless Close is called.
func NewTimescaleFromDSN(ctx context.Context, dsn string) (*Timescale, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Timescale{pool: pool}, nil
}

var snapshotColumns = []string{
	"contestant_id", "submission_id", "window_start", "duration_ns",
	"count", "rejected", "timeouts", "tps",
	"p50_ns", "p90_ns", "p99_ns", "p999_ns",
}

func (w *Timescale) WriteSnapshots(ctx context.Context, snaps []windowing.Snapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(snaps))
	for _, s := range snaps {
		rows = append(rows, []any{
			s.ContestantID, s.SubmissionID, s.WindowStart, s.Duration.Nanoseconds(),
			s.Count, s.Rejected, s.Timeouts, s.TPS,
			s.P50Ns, s.P90Ns, s.P99Ns, s.P999Ns,
		})
	}
	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"telemetry_snapshots"},
		snapshotColumns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("copy %d rows: %w", len(rows), err)
	}
	return nil
}

func (w *Timescale) Close() error {
	w.pool.Close()
	return nil
}

var _ Writer = (*Timescale)(nil)
