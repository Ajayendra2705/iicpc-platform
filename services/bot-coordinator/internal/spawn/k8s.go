package spawn

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sConfig configures the Kubernetes Job spawner.
type K8sConfig struct {
	Namespace      string // namespace for bot-worker Jobs
	BotWorkerImage string // e.g. "localhost:5000/bot-worker:latest"
	KubeConfigPath string // empty → in-cluster config
}

// K8sSpawner creates and manages bot-worker Kubernetes Jobs.
type K8sSpawner struct {
	client    kubernetes.Interface
	namespace string
	image     string
}

func NewK8sSpawner(cfg K8sConfig) (*K8sSpawner, error) {
	var restCfg *rest.Config
	var err error
	if cfg.KubeConfigPath != "" {
		restCfg, err = clientcmd.BuildConfigFromFlags("", cfg.KubeConfigPath)
	} else {
		restCfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}
	cli, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}
	return &K8sSpawner{client: cli, namespace: cfg.Namespace, image: cfg.BotWorkerImage}, nil
}

func (s *K8sSpawner) Start(ctx context.Context, benchmarkID string, spec BenchmarkSpec) error {
	job := s.buildJob(benchmarkID, spec)
	_, err := s.client.BatchV1().Jobs(s.namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create job %s: %w", benchmarkID, err)
	}
	return nil
}

func (s *K8sSpawner) Stop(ctx context.Context, benchmarkID string) error {
	propagation := metav1.DeletePropagationForeground
	err := s.client.BatchV1().Jobs(s.namespace).Delete(ctx, jobName(benchmarkID), metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil {
		return fmt.Errorf("delete job %s: %w", benchmarkID, err)
	}
	return nil
}

func (s *K8sSpawner) Status(ctx context.Context, benchmarkID string) (JobStatus, error) {
	job, err := s.client.BatchV1().Jobs(s.namespace).Get(ctx, jobName(benchmarkID), metav1.GetOptions{})
	if err != nil {
		return JobStatus{}, fmt.Errorf("get job %s: %w", benchmarkID, err)
	}
	phase := "running"
	if job.Status.Active == 0 && job.Status.Succeeded > 0 {
		phase = "succeeded"
	} else if job.Status.Failed > 0 {
		phase = "failed"
	}
	return JobStatus{
		Active:    job.Status.Active,
		Succeeded: job.Status.Succeeded,
		Failed:    job.Status.Failed,
		Phase:     phase,
	}, nil
}

func (s *K8sSpawner) buildJob(benchmarkID string, spec BenchmarkSpec) *batchv1.Job {
	workers := max(int32(spec.NumWorkers), 1)
	ttl := int32(300)

	env := []corev1.EnvVar{
		{Name: "TARGET_URL", Value: spec.TargetURL},
		{Name: "ORDERS_PER_SECOND", Value: fmt.Sprintf("%.0f", spec.OrdersPerSecond)},
		{Name: "PROTOCOL", Value: spec.Protocol},
		{Name: "ARRIVAL_MODE", Value: spec.ArrivalMode},
		{Name: "NUM_WORKERS", Value: "1"},
	}
	if spec.DurationSeconds > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "DURATION",
			Value: fmt.Sprintf("%d", spec.DurationSeconds),
		})
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName(benchmarkID),
			Namespace: s.namespace,
			Labels: map[string]string{
				"app":           "bot-worker",
				"benchmark-id":  benchmarkID,
				"submission-id": spec.SubmissionID,
			},
		},
		Spec: batchv1.JobSpec{
			Parallelism:             &workers,
			Completions:             &workers,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":          "bot-worker",
						"benchmark-id": benchmarkID,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "bot-worker",
							Image: s.image,
							Env:   env,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	if spec.DurationSeconds > 0 {
		deadline := int64(spec.DurationSeconds + 60)
		job.Spec.ActiveDeadlineSeconds = &deadline
	}

	return job
}

func jobName(benchmarkID string) string {
	return "bot-" + benchmarkID
}

var _ JobSpawner = (*K8sSpawner)(nil)
