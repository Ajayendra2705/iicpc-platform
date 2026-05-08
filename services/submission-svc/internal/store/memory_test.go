package store

import (
	"testing"
	"time"
)

func TestMemoryCreateAndGet(t *testing.T) {
	m := NewMemory()
	s := Submission{
		ID:           "abc",
		ContestantID: "team1",
		Language:     LangGo,
		Status:       StatusUploaded,
		CreatedAt:    time.Now(),
	}
	if err := m.Create(s); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, ok := m.Get("abc")
	if !ok {
		t.Fatal("missing after create")
	}
	if got.Language != LangGo {
		t.Errorf("language: %s", got.Language)
	}
}

func TestMemoryDuplicate(t *testing.T) {
	m := NewMemory()
	s := Submission{ID: "x"}
	_ = m.Create(s)
	if err := m.Create(s); err != ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestMemoryUpdateStatus(t *testing.T) {
	m := NewMemory()
	_ = m.Create(Submission{ID: "x", Status: StatusUploaded})
	if err := m.UpdateStatus("x", StatusReady, "registry/x:1", ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := m.Get("x")
	if got.Status != StatusReady || got.ImageURI != "registry/x:1" {
		t.Errorf("not updated: %+v", got)
	}
}

func TestMemoryListFilterAndLimit(t *testing.T) {
	m := NewMemory()
	now := time.Now()
	_ = m.Create(Submission{ID: "1", ContestantID: "a", CreatedAt: now.Add(-3 * time.Second)})
	_ = m.Create(Submission{ID: "2", ContestantID: "a", CreatedAt: now.Add(-2 * time.Second)})
	_ = m.Create(Submission{ID: "3", ContestantID: "b", CreatedAt: now.Add(-1 * time.Second)})

	all := m.List("", 0)
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}
	if all[0].ID != "3" {
		t.Errorf("expected newest first, got %s", all[0].ID)
	}

	teamA := m.List("a", 0)
	if len(teamA) != 2 {
		t.Errorf("expected 2 for team a, got %d", len(teamA))
	}

	limited := m.List("", 1)
	if len(limited) != 1 {
		t.Errorf("expected 1, got %d", len(limited))
	}
}
