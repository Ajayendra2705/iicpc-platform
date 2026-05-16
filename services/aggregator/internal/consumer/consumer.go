// Package consumer reads OrderEvents from Redpanda/Kafka and dispatches them
// to the aggregator. Both real (kafka-go) and Stub implementations exist.
package consumer

import (
	"context"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Consumer drives an event loop that calls handler for every consumed event.
// Returns nil when ctx is cancelled or an unrecoverable read error occurs.
type Consumer interface {
	Consume(ctx context.Context, handler func(*telemetryv1.OrderEvent)) error
	Close() error
}
