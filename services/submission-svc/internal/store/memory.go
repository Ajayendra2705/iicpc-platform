package store

import (
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("submission not found")
	ErrAlreadyExists = errors.New("submission already exists")
)

type Memory struct {
	mu          sync.RWMutex
	submissions map[string]Submission
}

func NewMemory() *Memory {
	return &Memory{submissions: make(map[string]Submission)}
}

func (m *Memory) Create(s Submission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.submissions[s.ID]; ok {
		return ErrAlreadyExists
	}
	m.submissions[s.ID] = s
	return nil
}

func (m *Memory) Get(id string) (Submission, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.submissions[id]
	return s, ok
}

func (m *Memory) UpdateStatus(id string, status Status, imageURI, buildError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.submissions[id]
	if !ok {
		return ErrNotFound
	}
	s.Status = status
	if imageURI != "" {
		s.ImageURI = imageURI
	}
	s.BuildError = buildError
	s.UpdatedAt = time.Now().UTC()
	m.submissions[id] = s
	return nil
}

func (m *Memory) List(contestantID string, limit int) []Submission {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Submission, 0, len(m.submissions))
	for _, s := range m.submissions {
		if contestantID != "" && s.ContestantID != contestantID {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
