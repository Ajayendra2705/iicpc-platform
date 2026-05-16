package producer_test

import (
	"context"
	"testing"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/telemetry-ingester/internal/producer"
)

func TestStubPublish(t *testing.T) {
	p := producer.NewStub()
	batch := []*telemetryv1.OrderEvent{
		{OrderId: "a"},
		{OrderId: "b"},
	}
	if err := p.Publish(context.Background(), batch); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if p.Published() != 2 {
		t.Errorf("Published: got %d want 2", p.Published())
	}
	if len(p.Batches()) != 1 || len(p.Batches()[0]) != 2 {
		t.Errorf("Batches shape unexpected: %+v", p.Batches())
	}
}

func TestStubMultipleBatches(t *testing.T) {
	p := producer.NewStub()
	for i := range 5 {
		_ = p.Publish(context.Background(), []*telemetryv1.OrderEvent{{OrderId: string(rune('a' + i))}})
	}
	if p.Published() != 5 {
		t.Errorf("Published: got %d want 5", p.Published())
	}
	if got := len(p.Batches()); got != 5 {
		t.Errorf("Batches: got %d want 5", got)
	}
}

func TestStubCloseIdempotent(t *testing.T) {
	p := producer.NewStub()
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close (2nd): %v", err)
	}
}
