package runner

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func contestantPod(name, cid, sid string, phase corev1.PodPhase, restarts int32) *corev1.Pod {
	labels := map[string]string{"app": "contestant-pod", "contestant-id": cid}
	if sid != "" {
		labels["submission-id"] = sid
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "iicpc-contestants",
			Labels:    labels,
		},
		Status: corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "contestant", RestartCount: restarts},
			},
		},
	}
}

func TestPodCrashes(t *testing.T) {
	tests := []struct {
		name        string
		pod         *corev1.Pod
		wantCID     string
		wantSID     string
		wantCrashes int64
		wantOK      bool
	}{
		{"failed-no-restarts", contestantPod("p1", "c1", "s1", corev1.PodFailed, 0), "c1", "s1", 1, true},
		{"crashloop-restarts", contestantPod("p2", "c1", "s1", corev1.PodRunning, 3), "c1", "s1", 3, true},
		{"healthy-running", contestantPod("p3", "c2", "s2", corev1.PodRunning, 0), "c2", "s2", 0, true},
		{"succeeded-clean", contestantPod("p4", "c2", "s2", corev1.PodSucceeded, 0), "c2", "s2", 0, true},
		{"legacy-no-submission", contestantPod("p5", "c3", "", corev1.PodFailed, 0), "c3", "", 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cid, sid, crashes, ok := podCrashes(tt.pod)
			if ok != tt.wantOK || cid != tt.wantCID || sid != tt.wantSID || crashes != tt.wantCrashes {
				t.Fatalf("got (%q,%q,%d,%v) want (%q,%q,%d,%v)",
					cid, sid, crashes, ok, tt.wantCID, tt.wantSID, tt.wantCrashes, tt.wantOK)
			}
		})
	}
}

func TestPodCrashesNoLabelIgnored(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "x"}, Status: corev1.PodStatus{Phase: corev1.PodFailed}}
	if _, _, _, ok := podCrashes(pod); ok {
		t.Fatal("pod without contestant-id label must be ignored")
	}
}

// TestCrashTrackerSumsPerSubmission proves a contestant's distinct attempts are
// counted separately: two submissions by c1 yield two rows, each with its own
// crash sum, rather than one blended contestant total.
func TestCrashTrackerSumsPerSubmission(t *testing.T) {
	tr := NewCrashTracker()
	tr.Observe("pod-a", "c1", "s1", 2) // c1 attempt s1, first pod: 2 restarts
	tr.Observe("pod-b", "c1", "s1", 1) // c1 attempt s1, replacement pod: +1
	tr.Observe("pod-c", "c1", "s2", 4) // c1 attempt s2: separate
	tr.Observe("pod-d", "c2", "s3", 5)
	tr.Observe("", "c3", "s9", 9)    // ignored (no pod name)
	tr.Observe("pod-e", "", "s9", 9) // ignored (no contestant)

	type key struct{ c, s string }
	got := map[key]int64{}
	for _, c := range tr.Report() {
		got[key{c.ContestantID, c.SubmissionID}] = c.Crashes
	}
	if got[key{"c1", "s1"}] != 3 {
		t.Errorf("c1/s1: got %d want 3", got[key{"c1", "s1"}])
	}
	if got[key{"c1", "s2"}] != 4 {
		t.Errorf("c1/s2: got %d want 4 (must not blend with s1)", got[key{"c1", "s2"}])
	}
	if got[key{"c2", "s3"}] != 5 {
		t.Errorf("c2/s3: got %d want 5", got[key{"c2", "s3"}])
	}
	if _, ok := got[key{"c3", "s9"}]; ok {
		t.Fatal("empty pod name should not be recorded")
	}
}

func TestCrashTrackerObserveIsMonotonicOverwrite(t *testing.T) {
	tr := NewCrashTracker()
	tr.Observe("pod-a", "c1", "s1", 1)
	tr.Observe("pod-a", "c1", "s1", 4) // same pod, later observation overwrites
	if got := tr.Report(); len(got) != 1 || got[0].Crashes != 4 {
		t.Fatalf("expected single row with 4 crashes, got %+v", got)
	}
}

// TestWatchCrashesViaFakeClient drives the real watch path against a fake
// clientset — no cluster required — and asserts a Failed contestant pod is
// counted as a crash, attributed to its submission.
func TestWatchCrashesViaFakeClient(t *testing.T) {
	client := fake.NewSimpleClientset()
	r := &Runner{client: client, cfg: Config{Namespace: "iicpc-contestants"}}
	tracker := NewCrashTracker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = r.WatchCrashes(ctx, tracker) }()

	// Let the watch establish before creating the pod (fake watches only
	// deliver events for actions after the watch starts).
	time.Sleep(100 * time.Millisecond)

	pod := contestantPod("contestant-sub1", "team-alpha", "sub-1", corev1.PodFailed, 0)
	if _, err := client.CoreV1().Pods("iicpc-contestants").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rep := tracker.Report()
		if len(rep) == 1 && rep[0].ContestantID == "team-alpha" && rep[0].SubmissionID == "sub-1" && rep[0].Crashes == 1 {
			return // success
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("crash not recorded within deadline: %+v", tracker.Report())
}
