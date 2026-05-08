package build

// Builder is the abstract interface the HTTP server calls into. The Stub
// implementation in stub.go is used until BuildKit wiring lands.
type Builder interface {
	Enqueue(submissionID string)
}
