package buildlog

import (
	"sync"
	"testing"
)

func TestAppendAndSince(t *testing.T) {
	b := NewBuffer(0)
	b.Append("s1", "status", "Build started")
	b.Append("s1", "stdout", "step 1/3")
	b.Append("s2", "stdout", "other submission") // must not bleed into s1

	all, latest := b.Since("s1", 0)
	if len(all) != 2 || latest != 2 {
		t.Fatalf("Since(0): got %d lines latest=%d, want 2/2", len(all), latest)
	}
	if all[0].Seq != 1 || all[0].Text != "Build started" || all[1].Seq != 2 {
		t.Fatalf("unexpected lines: %+v", all)
	}

	// Incremental poll: only lines after seq 1.
	rest, _ := b.Since("s1", 1)
	if len(rest) != 1 || rest[0].Text != "step 1/3" {
		t.Fatalf("Since(1): got %+v", rest)
	}

	// Isolation: s2 has its own sequence.
	s2, latest2 := b.Since("s2", 0)
	if len(s2) != 1 || latest2 != 1 {
		t.Fatalf("s2 isolation: got %d lines latest=%d", len(s2), latest2)
	}
}

func TestAppendIgnoresEmpty(t *testing.T) {
	b := NewBuffer(10)
	b.Append("", "stdout", "no submission")
	b.Append("s1", "stdout", "")
	if lines, latest := b.Since("s1", 0); len(lines) != 0 || latest != 0 {
		t.Fatalf("empty inputs should be ignored: %d/%d", len(lines), latest)
	}
}

func TestRetentionCap(t *testing.T) {
	b := NewBuffer(3)
	for range 10 {
		b.Append("s1", "stdout", "line")
	}
	lines, latest := b.Since("s1", 0)
	if len(lines) != 3 {
		t.Fatalf("cap: got %d lines, want 3", len(lines))
	}
	// Seq keeps climbing even as old lines are dropped; newest is line 10.
	if latest != 10 || lines[len(lines)-1].Seq != 10 {
		t.Fatalf("seq after cap: latest=%d lastSeq=%d, want 10/10", latest, lines[len(lines)-1].Seq)
	}
}

func TestConcurrentAppend(t *testing.T) {
	b := NewBuffer(0)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Append("s1", "stdout", "x")
		}()
	}
	wg.Wait()
	if _, latest := b.Since("s1", 0); latest != 50 {
		t.Fatalf("concurrent appends: latest=%d, want 50", latest)
	}
}
