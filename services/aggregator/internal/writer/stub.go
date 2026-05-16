package writer

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
)

// Stub records snapshot writes in memory; used in tests and dev mode.
type Stub struct {
	mu      sync.Mutex
	rows    []windowing.Snapshot
	written atomic.Uint64
}

func NewStub() *Stub { return &Stub{} }

func (w *Stub) WriteSnapshots(_ context.Context, snaps []windowing.Snapshot) error {
	w.mu.Lock()
	w.rows = append(w.rows, snaps...)
	w.mu.Unlock()
	w.written.Add(uint64(len(snaps))) //nolint:gosec
	return nil
}

func (w *Stub) Close() error { return nil }

// Written returns the total count of rows written across all batches.
func (w *Stub) Written() uint64 { return w.written.Load() }

// Rows returns a snapshot copy of all recorded rows.
func (w *Stub) Rows() []windowing.Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]windowing.Snapshot, len(w.rows))
	copy(cp, w.rows)
	return cp
}

var _ Writer = (*Stub)(nil)
