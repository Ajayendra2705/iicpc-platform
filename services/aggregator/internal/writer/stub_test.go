package writer_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/windowing"
	"github.com/Ajayendra2705/iicpc-platform/services/aggregator/internal/writer"
)

func snap(id string, count int64) windowing.Snapshot {
	return windowing.Snapshot{
		ContestantID: id,
		WindowStart:  time.Now(),
		Duration:     time.Second,
		Count:        count,
	}
}

func TestStubRecordsWrites(t *testing.T) {
	w := writer.NewStub()
	if err := w.WriteSnapshots(context.Background(), []windowing.Snapshot{snap("a", 1), snap("b", 2)}); err != nil {
		t.Fatalf("WriteSnapshots: %v", err)
	}
	if w.Written() != 2 {
		t.Errorf("written: got %d want 2", w.Written())
	}
	if len(w.Rows()) != 2 {
		t.Errorf("rows: got %d want 2", len(w.Rows()))
	}
}

func TestStubAccumulatesAcrossBatches(t *testing.T) {
	w := writer.NewStub()
	for i := range 5 {
		_ = w.WriteSnapshots(context.Background(), []windowing.Snapshot{snap("c", int64(i))})
	}
	if w.Written() != 5 {
		t.Errorf("written: got %d want 5", w.Written())
	}
}

func TestStubConcurrentWrites(t *testing.T) {
	w := writer.NewStub()
	var wg sync.WaitGroup
	const G = 50
	for range G {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.WriteSnapshots(context.Background(), []windowing.Snapshot{snap("x", 1)})
		}()
	}
	wg.Wait()
	if w.Written() != G {
		t.Errorf("concurrent written: got %d want %d", w.Written(), G)
	}
}

func TestStubCloseIsIdempotent(t *testing.T) {
	w := writer.NewStub()
	if err := w.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close (2nd): %v", err)
	}
}
