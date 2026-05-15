package spawn

import (
	"context"
	"errors"
	"sync"
)

// StubSpawner is an in-memory JobSpawner used in tests and the default dev mode.
type StubSpawner struct {
	mu   sync.Mutex
	jobs map[string]BenchmarkSpec
}

func NewStub() *StubSpawner {
	return &StubSpawner{jobs: make(map[string]BenchmarkSpec)}
}

func (s *StubSpawner) Start(_ context.Context, benchmarkID string, spec BenchmarkSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[benchmarkID]; exists {
		return errors.New("benchmark already running: " + benchmarkID)
	}
	s.jobs[benchmarkID] = spec
	return nil
}

func (s *StubSpawner) Stop(_ context.Context, benchmarkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[benchmarkID]; !exists {
		return errors.New("benchmark not found: " + benchmarkID)
	}
	delete(s.jobs, benchmarkID)
	return nil
}

func (s *StubSpawner) Status(_ context.Context, benchmarkID string) (JobStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spec, exists := s.jobs[benchmarkID]
	if !exists {
		return JobStatus{}, errors.New("benchmark not found: " + benchmarkID)
	}
	return JobStatus{
		Active: int32(spec.NumWorkers), //nolint:gosec
		Phase:  "running",
	}, nil
}

// Active returns the IDs of all running benchmarks (for tests).
func (s *StubSpawner) Active() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.jobs))
	for id := range s.jobs {
		ids = append(ids, id)
	}
	return ids
}

var _ JobSpawner = (*StubSpawner)(nil)
