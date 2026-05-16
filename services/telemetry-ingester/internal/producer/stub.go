package producer

import (
	"context"
	"sync"
	"sync/atomic"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Stub records published batches in memory; used in tests and dev mode.
type Stub struct {
	mu        sync.Mutex
	batches   [][]*telemetryv1.OrderEvent
	published atomic.Uint64
}

func NewStub() *Stub { return &Stub{} }

func (p *Stub) Publish(_ context.Context, batch []*telemetryv1.OrderEvent) error {
	cp := make([]*telemetryv1.OrderEvent, len(batch))
	copy(cp, batch)
	p.mu.Lock()
	p.batches = append(p.batches, cp)
	p.mu.Unlock()
	p.published.Add(uint64(len(batch))) //nolint:gosec
	return nil
}

func (p *Stub) Close() error { return nil }

// Published returns the total number of events successfully published.
func (p *Stub) Published() uint64 { return p.published.Load() }

// Batches returns a snapshot of all batches captured.
func (p *Stub) Batches() [][]*telemetryv1.OrderEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([][]*telemetryv1.OrderEvent, len(p.batches))
	copy(cp, p.batches)
	return cp
}

var _ Producer = (*Stub)(nil)
