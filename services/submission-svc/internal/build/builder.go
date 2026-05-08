package build

import "errors"

var ErrQueueFull = errors.New("build queue full")

// Builder is the abstract interface the HTTP server calls into. Returns
// ErrQueueFull when the implementation can't accept more work right now.
type Builder interface {
	Enqueue(submissionID string) error
}
