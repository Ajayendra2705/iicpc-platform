package runner

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Config holds all tunables for the sandbox runner.
type Config struct {
	Namespace        string
	RuntimeClass     string // "gvisor"; empty = omit (plain runc, for local dev)
	CPURequest       string
	CPULimit         string
	MemoryRequest    string
	MemoryLimit      string
	EphemeralLimit   string
	ReadinessTimeout time.Duration
}

// PodHandle is returned after a successful spawn.
type PodHandle struct {
	Name  string
	PodIP string
	Phase corev1.PodPhase
}

// Runner spawns and manages contestant pods via the K8s API.
type Runner struct {
	client kubernetes.Interface
	cfg    Config
}

// New builds a Runner using in-cluster config, falling back to kubeconfig.
func New(cfg Config) (*Runner, error) {
	kc, err := buildClient()
	if err != nil {
		return nil, err
	}
	return &Runner{client: kc, cfg: cfg}, nil
}

func buildClient() (kubernetes.Interface, error) {
	rc, err := rest.InClusterConfig()
	if err == nil {
		return kubernetes.NewForConfig(rc)
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cc, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}
	return kubernetes.NewForConfig(cc)
}

// Spawn creates the contestant pod and waits until it is Ready.
func (r *Runner) Spawn(ctx context.Context, submissionID, contestantID, imageURI string, runtimePort int32) (*PodHandle, error) {
	pod := r.podSpec(submissionID, contestantID, imageURI, runtimePort)
	created, err := r.client.CoreV1().Pods(r.cfg.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create pod: %w", err)
	}
	ip, err := r.waitReady(ctx, created.Name)
	if err != nil {
		return nil, err
	}
	return &PodHandle{Name: created.Name, PodIP: ip, Phase: corev1.PodRunning}, nil
}

// waitReady watches the pod until the Ready condition is true or the context deadline fires.
func (r *Runner) waitReady(ctx context.Context, name string) (string, error) {
	wctx, cancel := context.WithTimeout(ctx, r.cfg.ReadinessTimeout)
	defer cancel()

	w, err := r.client.CoreV1().Pods(r.cfg.Namespace).Watch(wctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + name,
	})
	if err != nil {
		return "", fmt.Errorf("watch pod %s: %w", name, err)
	}
	defer w.Stop()

	for event := range w.ResultChan() {
		if event.Type == watch.Error {
			return "", fmt.Errorf("watch error for pod %s", name)
		}
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			continue
		}
		if isPodReady(pod) {
			return pod.Status.PodIP, nil
		}
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return "", fmt.Errorf("pod %s terminated early: phase=%s", name, pod.Status.Phase)
		}
	}
	return "", fmt.Errorf("pod %s not ready within %s", name, r.cfg.ReadinessTimeout)
}

func isPodReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// Teardown deletes the contestant pod. Not-found is treated as success.
func (r *Runner) Teardown(ctx context.Context, podName string) error {
	err := r.client.CoreV1().Pods(r.cfg.Namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}

// Status returns current phase and IP for an existing pod.
func (r *Runner) Status(ctx context.Context, podName string) (*PodHandle, error) {
	pod, err := r.client.CoreV1().Pods(r.cfg.Namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod %s: %w", podName, err)
	}
	return &PodHandle{
		Name:  pod.Name,
		PodIP: pod.Status.PodIP,
		Phase: pod.Status.Phase,
	}, nil
}
