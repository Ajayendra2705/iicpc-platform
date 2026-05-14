package build

import "context"

// SandboxStarter is called after a successful build to spawn the contestant
// container. Nil means "skip" (no sandbox configured).
type SandboxStarter interface {
	Start(ctx context.Context, submissionID, contestantID, imageURI string) error
}
