// Package producer publishes telemetry batches to a downstream stream
// (Redpanda/Kafka in prod, in-memory stub in tests).
package producer

import (
	"context"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Producer publishes batches of OrderEvents. Implementations must be safe
// for concurrent Publish calls.
type Producer interface {
	Publish(ctx context.Context, batch []*telemetryv1.OrderEvent) error
	Close() error
}
