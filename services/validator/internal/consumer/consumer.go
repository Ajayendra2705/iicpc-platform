// Package consumer reads OrderEvents from Redpanda/Kafka and feeds the
// validator's replay engine.
package consumer

import (
	"context"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

type Consumer interface {
	Consume(ctx context.Context, handler func(*telemetryv1.OrderEvent)) error
	Close() error
}
