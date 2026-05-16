package store

import (
	"context"
	"sort"
	"sync"
)

// Stub keeps scores in memory; used in tests and dev mode without Redis.
type Stub struct {
	mu     sync.Mutex
	scores map[string]int64
}

func NewStub() *Stub { return &Stub{scores: make(map[string]int64)} }

func (s *Stub) Upsert(_ context.Context, id string, score int64) error {
	s.mu.Lock()
	s.scores[id] = score
	s.mu.Unlock()
	return nil
}

func (s *Stub) Top(_ context.Context, n int) ([]Entry, error) {
	s.mu.Lock()
	entries := make([]Entry, 0, len(s.scores))
	for id, score := range s.scores {
		entries = append(entries, Entry{ContestantID: id, Score: score})
	}
	s.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Score > entries[j].Score })
	if n > 0 && n < len(entries) {
		entries = entries[:n]
	}
	return entries, nil
}

func (s *Stub) Close() error { return nil }

var _ Store = (*Stub)(nil)
