package runner

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
)

// CrashCount is one contestant's accumulated crash count, served over HTTP and
// consumed by leaderboard-svc as the stability "crashes" penalty input.
type CrashCount struct {
	ContestantID string `json:"contestant_id"`
	Crashes      int64  `json:"crashes"`
}

// CrashTracker accumulates contestant-pod crash signals. It keys observations
// per pod (not per contestant) so that a replaced pod doesn't erase the history
// of an earlier one; Report sums per contestant. Safe for concurrent use.
type CrashTracker struct {
	mu   sync.Mutex
	pods map[string]podCrash // pod name -> latest observation
}

type podCrash struct {
	contestantID string
	crashes      int64
}

// NewCrashTracker returns an empty tracker.
func NewCrashTracker() *CrashTracker {
	return &CrashTracker{pods: make(map[string]podCrash)}
}

// Observe records the latest crash count for one pod. Crash counts are
// monotonic within a pod's lifetime, so overwriting with the latest value is
// correct.
func (t *CrashTracker) Observe(podName, contestantID string, crashes int64) {
	if podName == "" || contestantID == "" {
		return
	}
	t.mu.Lock()
	t.pods[podName] = podCrash{contestantID: contestantID, crashes: crashes}
	t.mu.Unlock()
}

// Report returns the summed crash count per contestant.
func (t *CrashTracker) Report() []CrashCount {
	t.mu.Lock()
	defer t.mu.Unlock()
	sums := make(map[string]int64, len(t.pods))
	for _, pc := range t.pods {
		sums[pc.contestantID] += pc.crashes
	}
	out := make([]CrashCount, 0, len(sums))
	for cid, n := range sums {
		out = append(out, CrashCount{ContestantID: cid, Crashes: n})
	}
	return out
}

// podCrashes extracts the crash signal from a contestant pod: the sum of
// container restart counts, plus one if the pod terminated in the Failed phase
// with no restarts. Contestant pods use RestartPolicy=Never, so a hard crash
// surfaces as a Failed phase (0 restarts) rather than a CrashLoopBackOff
// restart count; counting both keeps the signal correct if the policy changes.
func podCrashes(pod *corev1.Pod) (contestantID string, crashes int64, ok bool) {
	cid := pod.Labels["contestant-id"]
	if cid == "" {
		return "", 0, false
	}
	var restarts int64
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += int64(cs.RestartCount)
	}
	if pod.Status.Phase == corev1.PodFailed && restarts == 0 {
		restarts = 1
	}
	return cid, restarts, true
}
