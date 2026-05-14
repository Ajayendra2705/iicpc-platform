package build

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/store"
)

// Stub builder for local dev / tests without Docker. Day 3 introduces the
// real BuildKit-driven implementation; this remains for fast iteration when
// the Docker daemon isn't available.
type Stub struct {
	repo    store.Repository
	log     *slog.Logger
	queue   chan job
	sandbox SandboxStarter
}

type job struct {
	submissionID string
}

func NewStub(repo store.Repository, log *slog.Logger) *Stub {
	return NewStubWithSandbox(repo, log, nil)
}

func NewStubWithSandbox(repo store.Repository, log *slog.Logger, sandbox SandboxStarter) *Stub {
	if log == nil {
		log = slog.Default()
	}
	return &Stub{
		repo:    repo,
		log:     log.With("component", "build.stub"),
		queue:   make(chan job, 64),
		sandbox: sandbox,
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
			s.process(ctx, j)
		}
	}
}

func (s *Stub) process(ctx context.Context, j job) {
	sub, ok := s.repo.Get(j.submissionID)
	if !ok {
		s.log.Warn("submission not found", "submission_id", j.submissionID)
		return
	}

	if err := s.repo.UpdateStatus(j.submissionID, store.StatusBuilding, "", ""); err != nil {
		s.log.Error("update status building", "submission_id", j.submissionID, "err", err)
		return
	}

	imageURI := fmt.Sprintf("localhost:5000/contestants/%s:latest", j.submissionID)
	if err := s.repo.UpdateStatus(j.submissionID, store.StatusReady, imageURI, ""); err != nil {
		s.log.Error("update status ready", "submission_id", j.submissionID, "err", err)
		return
	}
	s.log.Info("build complete", "submission_id", j.submissionID, "image_uri", imageURI)

	if s.sandbox != nil {
		if err := s.sandbox.Start(ctx, j.submissionID, sub.ContestantID, imageURI); err != nil {
			s.log.Error("sandbox start failed", "submission_id", j.submissionID, "err", err)
		}
	}
}
