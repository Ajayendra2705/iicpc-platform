// Package buildlog holds bounded, per-submission build-log lines in memory so
// the UI can poll real `docker build`/`push` output instead of a synthetic
// placeholder sequence. It is deliberately in-process and best-effort: lines
// are capped per submission and lost on restart, which is fine for the
// live-build view (durable status still lives on the submission record).
package buildlog

import (
	"sync"
	"time"
)

// Line is one captured build-log line. Seq is monotonic per submission so the
// UI can poll incrementally with ?since=<lastSeq>.
type Line struct {
	Seq    int64  `json:"seq"`
	Stream string `json:"stream"` // stdout | stderr | status
	Text   string `json:"text"`
	AtMs   int64  `json:"at_ms"`
}

// Buffer is a concurrency-safe ring of recent log lines per submission.
type Buffer struct {
	mu       sync.Mutex
	lines    map[string][]Line
	seq      map[string]int64
	maxLines int
}

// NewBuffer returns a Buffer retaining at most maxLines per submission
// (<=0 defaults to 2000).
func NewBuffer(maxLines int) *Buffer {
	if maxLines <= 0 {
		maxLines = 2000
	}
	return &Buffer{
		lines:    make(map[string][]Line),
		seq:      make(map[string]int64),
		maxLines: maxLines,
	}
}

// Append records one line for a submission. Empty submission/text is ignored.
func (b *Buffer) Append(submissionID, stream, text string) {
	if submissionID == "" || text == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq[submissionID]++
	ln := Line{Seq: b.seq[submissionID], Stream: stream, Text: text, AtMs: time.Now().UnixMilli()}
	cur := append(b.lines[submissionID], ln)
	if len(cur) > b.maxLines {
		cur = cur[len(cur)-b.maxLines:]
	}
	b.lines[submissionID] = cur
}

// Since returns the lines for a submission with Seq strictly greater than
// since, plus the latest seq assigned (so the caller can poll incrementally).
func (b *Buffer) Since(submissionID string, since int64) (lines []Line, latest int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	all := b.lines[submissionID]
	out := make([]Line, 0, len(all))
	for _, ln := range all {
		if ln.Seq > since {
			out = append(out, ln)
		}
	}
	return out, b.seq[submissionID]
}
