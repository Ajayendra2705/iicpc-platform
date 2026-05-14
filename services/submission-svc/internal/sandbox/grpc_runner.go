package sandbox

import (
	"context"
	"fmt"

	sandboxv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/sandbox/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultRuntimePort = 9100

// GRPCRunner calls sandbox-runner over gRPC after a successful build.
type GRPCRunner struct {
	client sandboxv1.SandboxRunnerClient
	conn   *grpc.ClientConn
}

func NewGRPC(addr string) (*GRPCRunner, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("sandbox grpc dial %q: %w", addr, err)
	}
	return &GRPCRunner{client: sandboxv1.NewSandboxRunnerClient(conn), conn: conn}, nil
}

func (r *GRPCRunner) Close() error { return r.conn.Close() }

func (r *GRPCRunner) Start(ctx context.Context, submissionID, contestantID, imageURI string) error {
	_, err := r.client.RunSandbox(ctx, &sandboxv1.RunSandboxRequest{
		SubmissionId: submissionID,
		ContestantId: contestantID,
		ImageUri:     imageURI,
		RuntimePort:  defaultRuntimePort,
	})
	return err
}
