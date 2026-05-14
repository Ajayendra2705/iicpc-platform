package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"

	sandboxv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/sandbox/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/sandbox-runner/internal/runner"
)

// Server implements the SandboxRunner gRPC service.
type Server struct {
	sandboxv1.UnimplementedSandboxRunnerServer
	r *runner.Runner
}

func New(r *runner.Runner) *Server {
	return &Server{r: r}
}

func (s *Server) RunSandbox(ctx context.Context, req *sandboxv1.RunSandboxRequest) (*sandboxv1.RunSandboxResponse, error) {
	if req.SubmissionId == "" {
		return nil, status.Error(codes.InvalidArgument, "submission_id required")
	}
	if req.ImageUri == "" {
		return nil, status.Error(codes.InvalidArgument, "image_uri required")
	}
	port := req.RuntimePort
	if port == 0 {
		port = 9100
	}

	handle, err := s.r.Spawn(ctx, req.SubmissionId, req.ContestantId, req.ImageUri, port)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "spawn pod: %v", err)
	}

	return &sandboxv1.RunSandboxResponse{
		RunId:       handle.Name,
		PodIp:       handle.PodIP,
		RuntimePort: port,
		Phase:       toProtoPhase(handle.Phase),
	}, nil
}

func (s *Server) StopSandbox(ctx context.Context, req *sandboxv1.StopSandboxRequest) (*sandboxv1.StopSandboxResponse, error) {
	if req.RunId == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id required")
	}
	if err := s.r.Teardown(ctx, req.RunId); err != nil {
		return nil, status.Errorf(codes.Internal, "teardown pod: %v", err)
	}
	return &sandboxv1.StopSandboxResponse{Ok: true}, nil
}

func (s *Server) GetSandboxStatus(ctx context.Context, req *sandboxv1.GetSandboxStatusRequest) (*sandboxv1.GetSandboxStatusResponse, error) {
	if req.RunId == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id required")
	}
	handle, err := s.r.Status(ctx, req.RunId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get pod status: %v", err)
	}
	return &sandboxv1.GetSandboxStatusResponse{
		RunId: handle.Name,
		PodIp: handle.PodIP,
		Phase: toProtoPhase(handle.Phase),
	}, nil
}

func toProtoPhase(p corev1.PodPhase) sandboxv1.SandboxPhase {
	switch p {
	case corev1.PodPending:
		return sandboxv1.SandboxPhase_SANDBOX_PHASE_PENDING
	case corev1.PodRunning:
		return sandboxv1.SandboxPhase_SANDBOX_PHASE_RUNNING
	case corev1.PodSucceeded:
		return sandboxv1.SandboxPhase_SANDBOX_PHASE_SUCCEEDED
	case corev1.PodFailed:
		return sandboxv1.SandboxPhase_SANDBOX_PHASE_FAILED
	default:
		return sandboxv1.SandboxPhase_SANDBOX_PHASE_UNSPECIFIED
	}
}
