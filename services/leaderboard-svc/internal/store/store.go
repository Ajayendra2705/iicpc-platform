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
	Upsert(ctx context.Context, contestantID string, score int64) error
	Top(ctx context.Context, n int) ([]Entry, error)
	Close() error
}
