package build

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/iicpc/platform/services/submission-svc/internal/store"
)

// Stub builder. Day 3 will replace with real BuildKit invocation.
// For now: simulate build delay, mark image URI, transition status.
type Stub struct {
	repo  store.Repository
	queue chan job
}

type job struct {
	submissionID string
}

func NewStub(repo store.Repository) *Stub {
	return &Stub{
		repo:  repo,
		queue: make(chan job, 64),
	}
}

func (s *Stub) Enqueue(submissionID string) {
	select {
	case s.queue <- job{submissionID: submissionID}:
	default:
		log.Printf("build queue full, dropping submission %s", submissionID)
	}
}

func (s *Stub) Run(ctx context.Context) {
	log.Println("build worker (stub) started")
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-s.queue:
			s.process(j)
		}
	}
}

func (s *Stub) process(j job) {
	if err := s.repo.UpdateStatus(j.submissionID, store.StatusBuilding, "", ""); err != nil {
		log.Printf("update status building: %v", err)
		return
	}

	time.Sleep(2 * time.Second)

	imageURI := fmt.Sprintf("registry.iicpc.local/contestants/%s:latest", j.submissionID)
	if err := s.repo.UpdateStatus(j.submissionID, store.StatusReady, imageURI, ""); err != nil {
		log.Printf("update status ready: %v", err)
		return
	}
	log.Printf("build complete (stub) for %s -> %s", j.submissionID, imageURI)
}
