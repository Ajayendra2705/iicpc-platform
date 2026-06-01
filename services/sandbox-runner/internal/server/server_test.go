package server

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	sandboxv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/sandbox/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/sandbox-runner/internal/runner"
)

func testServer(objects ...*corev1.Pod) *Server {
	fc := fake.NewSimpleClientset()
	for _, p := range objects {
		_, _ = fc.CoreV1().Pods("iicpc-contestants").Create(context.Background(), p, metav1.CreateOptions{})
	}
	r := runner.NewWithClient(fc, runner.Config{
		Namespace:        "iicpc-contestants",
		ReadinessTimeout: time.Second,
	})
	return New(r)
}

func codeOf(t *testing.T, err error) codes.Code {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %T: %v", err, err)
	}
	return st.Code()
}

func TestRunSandboxValidation(t *testing.T) {
	s := testServer()
	ctx := context.Background()

	if _, err := s.RunSandbox(ctx, &sandboxv1.RunSandboxRequest{ImageUri: "img:latest"}); codeOf(t, err) != codes.InvalidArgument {
		t.Fatalf("missing submission_id: want InvalidArgument, got %v", err)
	}
	if _, err := s.RunSandbox(ctx, &sandboxv1.RunSandboxRequest{SubmissionId: "sub-1"}); codeOf(t, err) != codes.InvalidArgument {
		t.Fatalf("missing image_uri: want InvalidArgument, got %v", err)
	}
}

func TestStopSandboxValidation(t *testing.T) {
	s := testServer()
	if _, err := s.StopSandbox(context.Background(), &sandboxv1.StopSandboxRequest{}); codeOf(t, err) != codes.InvalidArgument {
		t.Fatalf("missing run_id: want InvalidArgument, got %v", err)
	}
}

func TestStopSandboxNotFoundIsOK(t *testing.T) {
	s := testServer()
	resp, err := s.StopSandbox(context.Background(), &sandboxv1.StopSandboxRequest{RunId: "contestant-ghost"})
	if err != nil {
		t.Fatalf("tearing down a nonexistent pod should succeed, got %v", err)
	}
	if !resp.Ok {
		t.Fatal("expected Ok=true on idempotent teardown")
	}
}

func TestGetSandboxStatusValidation(t *testing.T) {
	s := testServer()
	if _, err := s.GetSandboxStatus(context.Background(), &sandboxv1.GetSandboxStatusRequest{}); codeOf(t, err) != codes.InvalidArgument {
		t.Fatalf("missing run_id: want InvalidArgument, got %v", err)
	}
}

func TestGetSandboxStatusReportsRunningPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "contestant-sub-7", Namespace: "iicpc-contestants"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.2.7"},
	}
	s := testServer(pod)

	resp, err := s.GetSandboxStatus(context.Background(), &sandboxv1.GetSandboxStatusRequest{RunId: "contestant-sub-7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PodIp != "10.0.2.7" {
		t.Fatalf("expected pod IP 10.0.2.7, got %q", resp.PodIp)
	}
	if resp.Phase != sandboxv1.SandboxPhase_SANDBOX_PHASE_RUNNING {
		t.Fatalf("expected RUNNING phase, got %v", resp.Phase)
	}
}

func TestGetSandboxStatusMissingPodErrors(t *testing.T) {
	s := testServer()
	if _, err := s.GetSandboxStatus(context.Background(), &sandboxv1.GetSandboxStatusRequest{RunId: "contestant-absent"}); codeOf(t, err) != codes.Internal {
		t.Fatalf("status of missing pod: want Internal, got %v", err)
	}
}

func TestToProtoPhase(t *testing.T) {
	cases := map[corev1.PodPhase]sandboxv1.SandboxPhase{
		corev1.PodPending:   sandboxv1.SandboxPhase_SANDBOX_PHASE_PENDING,
		corev1.PodRunning:   sandboxv1.SandboxPhase_SANDBOX_PHASE_RUNNING,
		corev1.PodSucceeded: sandboxv1.SandboxPhase_SANDBOX_PHASE_SUCCEEDED,
		corev1.PodFailed:    sandboxv1.SandboxPhase_SANDBOX_PHASE_FAILED,
		corev1.PodUnknown:   sandboxv1.SandboxPhase_SANDBOX_PHASE_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := toProtoPhase(in); got != want {
			t.Errorf("toProtoPhase(%s) = %v, want %v", in, got, want)
		}
	}
}
