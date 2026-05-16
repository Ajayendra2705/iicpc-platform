package store_test

import (
	"context"
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/store"
)

func TestUpsertAndTopOrdered(t *testing.T) {
	s := store.NewStub()
	ctx := context.Background()
	_ = s.Upsert(ctx, "a", 100)
	_ = s.Upsert(ctx, "b", 300)
	_ = s.Upsert(ctx, "c", 200)

	top, err := s.Top(ctx, 0)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	want := []string{"b", "c", "a"}
	if len(top) != 3 {
		t.Fatalf("Top len: got %d want 3", len(top))
	}
	for i, e := range top {
		if e.ContestantID != want[i] {
			t.Errorf("Top[%d]: got %s want %s", i, e.ContestantID, want[i])
		}
	}
}

func TestTopRespectsLimit(t *testing.T) {
	s := store.NewStub()
	ctx := context.Background()
	for i, id := range []string{"a", "b", "c", "d", "e"} {
		_ = s.Upsert(ctx, id, int64(100-i))
	}
	top, _ := s.Top(ctx, 3)
	if len(top) != 3 {
		t.Errorf("Top: got %d want 3", len(top))
	}
}

func TestUpsertOverwrites(t *testing.T) {
	s := store.NewStub()
	ctx := context.Background()
	_ = s.Upsert(ctx, "a", 100)
	_ = s.Upsert(ctx, "a", 200)
	top, _ := s.Top(ctx, 0)
	if len(top) != 1 || top[0].Score != 200 {
		t.Errorf("upsert overwrite: got %+v", top)
	}
}
