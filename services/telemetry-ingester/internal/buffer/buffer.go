// Package buffer provides a bounded, lock-light queue for OrderEvent telemetry.
// Producers (gRPC stream handlers) call Push concurrently; a single consumer
// reads from Recv() and batches events before publishing to Redpanda.
package buffer

import (
	"sync/atomic"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Buffer is a bounded MPSC queue. Push is non-blocking and drops on full.
type Buffer struct {
	ch      chan *telemetryv1.OrderEvent
	pushed  atomic.Uint64
	dropped atomic.Uint64
	closed  atomic.Bool
}

// Stats reports lifetime push/drop counts for backpressure observability.
type Stats struct {
	Pushed  uint64
	Dropped uint64
}

// New creates a buffer with the given capacity (events).
func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = 1024
	}
	return &Buffer{ch: make(chan *telemetryv1.OrderEvent, capacity)}
}

// Push enqueues an event. Returns false if the buffer is full or closed.
func (b *Buffer) Push(e *telemetryv1.OrderEvent) bool {
	if b.closed.Load() {
		return false
	}
	select {
	case b.ch <- e:
		b.pushed.Add(1)
		return true
	default:
		b.dropped.Add(1)
		return false
	}
}

// Recv returns the read side of the channel for consumers to drain directly.
func (b *Buffer) Recv() <-chan *telemetryv1.OrderEvent { return b.ch }

// Stats returns lifetime counters.
func (b *Buffer) Stats() Stats {
	return Stats{Pushed: b.pushed.Load(), Dropped: b.dropped.Load()}
}

// Close marks the buffer closed and shuts the channel; consumers see Recv close.
func (b *Buffer) Close() {
	if b.closed.CompareAndSwap(false, true) {
		close(b.ch)
	}
}
