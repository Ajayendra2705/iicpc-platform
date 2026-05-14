package runner

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// podSpec builds a hardened Pod spec for the given submission.
func (r *Runner) podSpec(submissionID, contestantID, imageURI string, runtimePort int32) *corev1.Pod {
	no := false
	yes := true
	var uid int64 = 65532

	var runtimeClassName *string
	if r.cfg.RuntimeClass != "" {
		rc := r.cfg.RuntimeClass
		runtimeClassName = &rc
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName(submissionID),
			Namespace: r.cfg.Namespace,
			Labels: map[string]string{
				"app":           "contestant-pod",
				"submission-id": submissionID,
				"contestant-id": contestantID,
				"iicpc.io/pool": "contestants",
			},
		},
		Spec: corev1.PodSpec{
			RuntimeClassName: runtimeClassName,
			RestartPolicy:    corev1.RestartPolicyNever,
			NodeSelector: map[string]string{
				"iicpc.io/pool": "contestants",
			},
			Containers: []corev1.Container{{
				Name:            "contestant",
				Image:           imageURI,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Ports: []corev1.ContainerPort{{
					Name:          "runtime",
					ContainerPort: runtimePort,
					Protocol:      corev1.ProtocolTCP,
				}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse(r.cfg.CPURequest),
						corev1.ResourceMemory:           resource.MustParse(r.cfg.MemoryRequest),
						corev1.ResourceEphemeralStorage: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse(r.cfg.CPULimit),
						corev1.ResourceMemory:           resource.MustParse(r.cfg.MemoryLimit),
						corev1.ResourceEphemeralStorage: resource.MustParse(r.cfg.EphemeralLimit),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &no,
					ReadOnlyRootFilesystem:   &yes,
					RunAsNonRoot:             &yes,
					RunAsUser:                &uid,
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
					SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/health",
							Port: intstr.FromInt32(runtimePort),
						},
					},
					InitialDelaySeconds: 2,
					PeriodSeconds:       3,
					FailureThreshold:    20,
				},
			}},
		},
	}
}

// podName derives a valid K8s pod name from a submission ID.
// Submission IDs are UUIDs (36 chars), so "contestant-" + id ≤ 47 chars < 63.
func podName(submissionID string) string {
	return "contestant-" + submissionID
}
