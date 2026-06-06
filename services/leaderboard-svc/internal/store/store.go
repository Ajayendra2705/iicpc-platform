// Package store holds the leaderboard ZSET state. Redis in prod, in-memory stub
// in tests/dev mode.
package store

import "context"

// Entry is one ranked row.
type Entry struct {
	ContestantID string `json:"contestant_id"`
	Score        int64  `json:"score"`
}

// Store persists per-contestant scores and answers top-N queries.
type Store interface {
	// Upsert sets a contestant's leaderboard score directly. Used for seeding
	// and legacy single-submission flows.
	Upsert(ctx context.Context, contestantID string, score int64) error
	// UpsertSubmission records the score of one submission (attempt) and sets
	// the contestant's leaderboard score to the best (max) across all of their
	// submissions. This is what implements best-submission ranking: a later,
	// worse attempt never lowers a contestant below an earlier, better one.
	UpsertSubmission(ctx context.Context, contestantID, submissionID string, score int64) error
	Top(ctx context.Context, n int) ([]Entry, error)
	Close() error
}
