package runner

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testConfig() Config {
	return Config{
		Namespace:        "iicpc-contestants",
		RuntimeClass:     "gvisor",
		CPURequest:       "500m",
		CPULimit:         "1000m",
		MemoryRequest:    "256Mi",
		MemoryLimit:      "512Mi",
		EphemeralLimit:   "2Gi",
		ReadinessTimeout: 5 * time.Second,
	}
}

func TestPodSpecRuntimeClass(t *testing.T) {
	r := &Runner{client: fake.NewSimpleClientset(), cfg: testConfig()}
	pod := r.podSpec("abc-123", "c1", "localhost:5000/img:latest", 9100)

	if pod.Spec.RuntimeClassName == nil || *pod.Spec.RuntimeClassName != "gvisor" {
		t.Fatalf("expected runtimeClassName gvisor, got %v", pod.Spec.RuntimeClassName)
	}
}

func TestPodSpecRuntimeClassOmittedWhenEmpty(t *testing.T) {
	cfg := testConfig()
	cfg.RuntimeClass = ""
	r := &Runner{client: fake.NewSimpleClientset(), cfg: cfg}
	pod := r.podSpec("abc-123", "c1", "localhost:5000/img:latest", 9100)

	if pod.Spec.RuntimeClassName != nil {
		t.Fatalf("expected nil runtimeClassName, got %v", *pod.Spec.RuntimeClassName)
	}
}

func TestPodSpecSecurityContext(t *testing.T) {
	r := &Runner{client: fake.NewSimpleClientset(), cfg: testConfig()}
	pod := r.podSpec("abc-123", "c1", "localhost:5000/img:latest", 9100)
	sc := pod.Spec.Containers[0].SecurityContext

	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Fatal("expected AllowPrivilegeEscalation=false")
	}
	if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		t.Fatal("expected ReadOnlyRootFilesystem=true")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Fatal("expected RunAsNonRoot=true")
	}
	if len(sc.Capabilities.Drop) == 0 || sc.Capabilities.Drop[0] != "ALL" {
		t.Fatal("expected capabilities.drop [ALL]")
	}
	if sc.SeccompProfile == nil || sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Fatal("expected seccompProfile RuntimeDefault")
	}
}

func TestPodSpecResourceLimits(t *testing.T) {
	r := &Runner{client: fake.NewSimpleClientset(), cfg: testConfig()}
	pod := r.podSpec("abc-123", "c1", "localhost:5000/img:latest", 9100)
	limits := pod.Spec.Containers[0].Resources.Limits

	if limits.Cpu().IsZero() {
		t.Fatal("expected non-zero CPU limit")
	}
	if limits.Memory().IsZero() {
		t.Fatal("expected non-zero memory limit")
	}
}

func TestPodSpecNodeSelector(t *testing.T) {
	r := &Runner{client: fake.NewSimpleClientset(), cfg: testConfig()}
	pod := r.podSpec("abc-123", "c1", "localhost:5000/img:latest", 9100)

	if pod.Spec.NodeSelector["iicpc.io/pool"] != "contestants" {
		t.Fatalf("expected nodeSelector iicpc.io/pool=contestants, got %v", pod.Spec.NodeSelector)
	}
}

func TestTeardownNotFound(t *testing.T) {
	r := &Runner{client: fake.NewSimpleClientset(), cfg: testConfig()}
	if err := r.Teardown(context.Background(), "nonexistent"); err != nil {
		t.Fatalf("teardown of nonexistent pod should not error: %v", err)
	}
}

func TestStatus(t *testing.T) {
	fc := fake.NewSimpleClientset()
	r := &Runner{client: fc, cfg: testConfig()}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contestant-sub-999",
			Namespace: "iicpc-contestants",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.1.42",
		},
	}
	if _, err := fc.CoreV1().Pods("iicpc-contestants").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	handle, err := r.Status(context.Background(), "contestant-sub-999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handle.PodIP != "10.0.1.42" {
		t.Fatalf("expected IP 10.0.1.42, got %s", handle.PodIP)
	}
	if handle.Phase != corev1.PodRunning {
		t.Fatalf("expected Running, got %s", handle.Phase)
	}
}

func TestPodName(t *testing.T) {
	name := podName("550e8400-e29b-41d4-a716-446655440000")
	if len(name) > 63 {
		t.Fatalf("pod name too long: %d chars", len(name))
	}
}
