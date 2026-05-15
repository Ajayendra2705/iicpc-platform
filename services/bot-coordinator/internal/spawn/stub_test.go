package spawn_test

import (
	"context"
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-coordinator/internal/spawn"
)

func spec(n int) spawn.BenchmarkSpec {
	return spawn.BenchmarkSpec{SubmissionID: "sub-1", TargetURL: "http://x", NumWorkers: n}
}

func TestStubStartStatus(t *testing.T) {
	s := spawn.NewStub()
	if err := s.Start(context.Background(), "b1", spec(5)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st, err := s.Status(context.Background(), "b1")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Active != 5 {
		t.Errorf("Active: got %d want 5", st.Active)
	}
	if st.Phase != "running" {
		t.Errorf("Phase: got %q want running", st.Phase)
	}
}

func TestStubStop(t *testing.T) {
	s := spawn.NewStub()
	_ = s.Start(context.Background(), "b1", spec(1))
	if err := s.Stop(context.Background(), "b1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := s.Status(context.Background(), "b1"); err == nil {
		t.Fatal("expected error after stop")
	}
}

func TestStubDuplicateStart(t *testing.T) {
	s := spawn.NewStub()
	_ = s.Start(context.Background(), "b1", spec(1))
	if err := s.Start(context.Background(), "b1", spec(1)); err == nil {
		t.Fatal("expected error on duplicate start")
	}
}

func TestStubStopNotFound(t *testing.T) {
	s := spawn.NewStub()
	if err := s.Stop(context.Background(), "missing"); err == nil {
		t.Fatal("expected error stopping non-existent benchmark")
	}
}

func TestStubStatusNotFound(t *testing.T) {
	s := spawn.NewStub()
	if _, err := s.Status(context.Background(), "missing"); err == nil {
		t.Fatal("expected error status of non-existent benchmark")
	}
}

func TestStubActive(t *testing.T) {
	s := spawn.NewStub()
	_ = s.Start(context.Background(), "b1", spec(1))
	_ = s.Start(context.Background(), "b2", spec(2))
	if got := len(s.Active()); got != 2 {
		t.Errorf("Active: got %d want 2", got)
	}
	_ = s.Stop(context.Background(), "b1")
	if got := len(s.Active()); got != 1 {
		t.Errorf("Active after stop: got %d want 1", got)
	}
}
