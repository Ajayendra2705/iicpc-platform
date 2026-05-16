package consumer_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/validator/internal/consumer"
)

func TestStubReplaysAllEvents(t *testing.T) {
	events := []*telemetryv1.OrderEvent{
		{OrderId: "a"}, {OrderId: "b"}, {OrderId: "c"},
	}
	c := consumer.NewStub(events)

	var got atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = c.Consume(ctx, func(_ *telemetryv1.OrderEvent) { got.Add(1) })
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if got.Load() != 3 {
		t.Errorf("handler invoked %d times, want 3", got.Load())
	}
}

func TestStubStopsOnCtxCancel(t *testing.T) {
	c := consumer.NewStub(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Consume(ctx, func(_ *telemetryv1.OrderEvent) {}); err != nil {
		t.Errorf("Consume: %v", err)
	}
}
