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

// TestUpsertSubmissionKeepsBest is the headline best-submission-ranking test:
// four attempts scoring 50, 40, 60, 30 must leave the contestant ranked on 60,
// not the latest (30) and not anything else.
func TestUpsertSubmissionKeepsBest(t *testing.T) {
	s := store.NewStub()
	ctx := context.Background()
	scores := []struct {
		sub   string
		score int64
	}{{"s1", 50}, {"s2", 40}, {"s3", 60}, {"s4", 30}}
	for _, x := range scores {
		if err := s.UpsertSubmission(ctx, "alice", x.sub, x.score); err != nil {
			t.Fatalf("UpsertSubmission(%s): %v", x.sub, err)
		}
	}
	top, _ := s.Top(ctx, 0)
	if len(top) != 1 {
		t.Fatalf("Top len: got %d want 1", len(top))
	}
	if top[0].ContestantID != "alice" || top[0].Score != 60 {
		t.Errorf("best-of: got %+v, want alice=60", top[0])
	}
}

// TestUpsertSubmissionRescoreLowersWhenBestRevised proves the leaderboard score
// is a recompute over current submission scores, not a sticky peak: if the
// best submission is re-scored downward (e.g. as more of its run is observed),
// the contestant's score follows.
func TestUpsertSubmissionRescoreLowersWhenBestRevised(t *testing.T) {
	s := store.NewStub()
	ctx := context.Background()
	_ = s.UpsertSubmission(ctx, "bob", "s1", 40)
	_ = s.UpsertSubmission(ctx, "bob", "s2", 70) // s2 is best
	if top, _ := s.Top(ctx, 0); top[0].Score != 70 {
		t.Fatalf("pre-rescore: got %d want 70", top[0].Score)
	}
	_ = s.UpsertSubmission(ctx, "bob", "s2", 50) // s2 revised down
	top, _ := s.Top(ctx, 0)
	if top[0].Score != 50 {
		t.Errorf("post-rescore: got %d want 50 (recomputed max of {40,50})", top[0].Score)
	}
}

// TestUpsertSubmissionBestAcrossContestants confirms ranking still orders by
// each contestant's best across multiple contestants.
func TestUpsertSubmissionBestAcrossContestants(t *testing.T) {
	s := store.NewStub()
	ctx := context.Background()
	_ = s.UpsertSubmission(ctx, "a", "a1", 30)
	_ = s.UpsertSubmission(ctx, "a", "a2", 90) // a best = 90
	_ = s.UpsertSubmission(ctx, "b", "b1", 80) // b best = 80
	top, _ := s.Top(ctx, 0)
	if len(top) != 2 || top[0].ContestantID != "a" || top[0].Score != 90 || top[1].Score != 80 {
		t.Errorf("ranking by best: got %+v", top)
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
