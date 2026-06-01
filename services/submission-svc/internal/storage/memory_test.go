package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestMemoryPutGetRoundTrip(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	payload := []byte("contestant-archive-bytes")

	uri, err := s.Put(ctx, "subs/abc.tar.gz", bytes.NewReader(payload), int64(len(payload)), "application/gzip")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if uri != "memory://subs/abc.tar.gz" {
		t.Fatalf("unexpected uri %q", uri)
	}

	rc, n, err := s.Get(ctx, "subs/abc.tar.gz")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	if n != int64(len(payload)) {
		t.Fatalf("expected size %d, got %d", len(payload), n)
	}
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, payload) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, payload)
	}
}

func TestMemoryGetMissing(t *testing.T) {
	s := NewMemory()
	if _, _, err := s.Get(context.Background(), "nope"); err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestMemoryPutOverwrites(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	_, _ = s.Put(ctx, "k", strings.NewReader("v1"), 2, "text/plain")
	_, _ = s.Put(ctx, "k", strings.NewReader("v2-longer"), 9, "text/plain")

	rc, n, err := s.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "v2-longer" || n != 9 {
		t.Fatalf("expected overwrite to v2-longer (9 bytes), got %q (%d)", got, n)
	}
}

// TestMemoryConcurrentAccess exercises the RWMutex under -race: many writers
// and readers on distinct keys must not data-race.
func TestMemoryConcurrentAccess(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i%26))
			if _, err := s.Put(ctx, key, strings.NewReader("x"), 1, "text/plain"); err != nil {
				t.Errorf("Put: %v", err)
			}
			if rc, _, err := s.Get(ctx, key); err == nil {
				_ = rc.Close()
			}
		}(i)
	}
	wg.Wait()
}
