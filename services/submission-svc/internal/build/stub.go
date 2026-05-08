package build

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iicpc/platform/services/submission-svc/internal/store"
)

// Stub builder for local dev / tests without Docker. Day 3 introduces the
// real BuildKit-driven implementation; this remains for fast iteration when
// the Docker daemon isn't available.
type Stub struct {
	repo  store.Repository
	log   *slog.Logger
	queue chan job
}

type job struct {
	submissionID string
}

func NewStub(repo store.Repository, log *slog.Logger) *Stub {
	if log == nil {
		log = slog.Default()
	}
	return &Stub{
		repo:  repo,
		log:   log.With("component", "build.stub"),
		queue: make(chan job, 64),
	}
}

func (s *Stub) Enqueue(submissionID string) error {
	select {
	case s.queue <- job{submissionID: submissionID}:
		return nil
	default:
		return ErrQueueFull
	}
}

func (s *Stub) Run(ctx context.Context) {
	s.log.Info("worker started")
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
		s.log.Error("update status building", "submission_id", j.submissionID, "err", err)
		return
	}

	time.Sleep(2 * time.Second)

	imageURI := fmt.Sprintf("registry.iicpc.local/contestants/%s:latest", j.submissionID)
	if err := s.repo.UpdateStatus(j.submissionID, store.StatusReady, imageURI, ""); err != nil {
		s.log.Error("update status ready", "submission_id", j.submissionID, "err", err)
		return
	}
	s.log.Info("build complete", "submission_id", j.submissionID, "image_uri", imageURI)
}
