package buffer_test

import (
	"sync"
	"testing"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/telemetry-ingester/internal/buffer"
)

func TestPushUpToCapacity(t *testing.T) {
	b := buffer.New(3)
	for range 3 {
		if !b.Push(&telemetryv1.OrderEvent{}) {
			t.Fatal("push within capacity returned false")
		}
	}
	s := b.Stats()
	if s.Pushed != 3 || s.Dropped != 0 {
		t.Errorf("stats: got pushed=%d dropped=%d", s.Pushed, s.Dropped)
	}
}

func TestPushDropsWhenFull(t *testing.T) {
	b := buffer.New(2)
	b.Push(&telemetryv1.OrderEvent{})
	b.Push(&telemetryv1.OrderEvent{})
	if b.Push(&telemetryv1.OrderEvent{}) {
		t.Fatal("push past capacity should return false")
	}
	s := b.Stats()
	if s.Dropped != 1 {
		t.Errorf("expected 1 drop, got %d", s.Dropped)
	}
}

func TestRecvReturnsPushed(t *testing.T) {
	b := buffer.New(4)
	for i := range 3 {
		b.Push(&telemetryv1.OrderEvent{OrderId: string(rune('a' + i))})
	}
	got := 0
	for range 3 {
		<-b.Recv()
		got++
	}
	if got != 3 {
		t.Errorf("recv: got %d events", got)
	}
}

func TestCloseStopsPush(t *testing.T) {
	b := buffer.New(4)
	b.Close()
	if b.Push(&telemetryv1.OrderEvent{}) {
		t.Fatal("push after close should return false")
	}
}

func TestCloseClosesRecv(t *testing.T) {
	b := buffer.New(4)
	b.Push(&telemetryv1.OrderEvent{})
	b.Close()
	<-b.Recv() // drain pushed event
	if _, ok := <-b.Recv(); ok {
		t.Fatal("recv after close+drain should be closed (ok=false)")
	}
}

func TestConcurrentPush(t *testing.T) {
	b := buffer.New(10_000)
	var wg sync.WaitGroup
	const G = 100
	const N = 100
	for range G {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range N {
				b.Push(&telemetryv1.OrderEvent{})
			}
		}()
	}
	wg.Wait()
	s := b.Stats()
	if s.Pushed != G*N {
		t.Errorf("pushed: got %d want %d", s.Pushed, G*N)
	}
}
