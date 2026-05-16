package consumer

import (
	"context"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Stub replays a fixed slice of events to the handler, then blocks until
// ctx is cancelled. Used in tests and dev mode without Kafka.
type Stub struct {
	events []*telemetryv1.OrderEvent
}

func NewStub(events []*telemetryv1.OrderEvent) *Stub {
	return &Stub{events: events}
}

func (c *Stub) Consume(ctx context.Context, handler func(*telemetryv1.OrderEvent)) error {
	for _, e := range c.events {
		if ctx.Err() != nil {
			return nil
		}
		handler(e)
	}
	<-ctx.Done()
	return nil
}

func (c *Stub) Close() error { return nil }

var _ Consumer = (*Stub)(nil)
