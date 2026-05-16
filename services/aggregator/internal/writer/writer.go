// Package writer persists rolled-up Snapshots to long-term storage
// (TimescaleDB in prod, in-memory stub in tests).
package writer

import (
	"context"

	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

// Writer persists a batch of windowed Snapshots.
// Implementations must be safe for concurrent use.
type Writer interface {
	WriteSnapshots(ctx context.Context, snaps []windowing.Snapshot) error
	Close() error
}
