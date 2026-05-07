package notifier

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// recordingClient implements eventsClient and records every Create call.
// Tests configure a `nextErr` function that returns the error (or nil) for
// each call, allowing per-attempt control of retry vs. permanent failure.
type recordingClient struct {
	mu       sync.Mutex
	calls    []*corev1.Event
	nextErr  func(callNum int) error // 0-indexed call number → error to return
	preCall  func()                  // optional hook called before each Create returns
}

func (r *recordingClient) Create(_ context.Context, ns string, ev *corev1.Event, _ metav1.CreateOptions) (*corev1.Event, error) {
	if r.preCall != nil {
		r.preCall()
	}
	r.mu.Lock()
	r.calls = append(r.calls, ev)
	n := len(r.calls) - 1
	r.mu.Unlock()
	if r.nextErr != nil {
		if err := r.nextErr(n); err != nil {
			return nil, err
		}
	}
	return ev, nil
}

func (r *recordingClient) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *recordingClient) callAt(i int) *corev1.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[i]
}

// newTestNotifier constructs a KubernetesEventNotifier wired to a recordingClient.
func newTestNotifier(client *recordingClient) *KubernetesEventNotifier {
	return newKubernetesEventNotifierWithClient(
		client,
		"ding-test",            // namespace
		"ding-pod",             // podName
		types.UID("pod-uid-1"), // podUID
		"node-1",               // nodeName
		"DingAlertFired",       // eventReason
		"Warning",              // eventType
		3,                      // maxAttempts
		10*time.Millisecond,    // initialBackoff (small for fast tests)
		nil,                    // collector
	)
}

func sampleAlert() evaluator.Alert {
	return evaluator.Alert{
		Rule:    "loss_spike",
		Metric:  "val_loss",
		Value:   12.4,
		Message: "val_loss spike: 12.4 on epoch 7",
		Labels:  map[string]string{"runner": "kubernetes"},
		Floats:  map[string]float64{},
		FiredAt: time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
	}
}

// waitForCalls polls until the recordingClient has at least n Create calls or
// timeout elapses. Returns true on success, false on timeout.
func waitForCalls(rc *recordingClient, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if rc.callCount() >= n {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return rc.callCount() >= n
}

func TestKubernetesEvent_PublishesEvent(t *testing.T) {
	rc := &recordingClient{}
	n := newTestNotifier(rc)
	defer n.Stop()

	if err := n.Send(sampleAlert()); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !waitForCalls(rc, 1, 1*time.Second) {
		t.Fatalf("expected 1 Create call, got %d", rc.callCount())
	}

	ev := rc.callAt(0)
	if ev.Reason != "DingAlertFired" {
		t.Errorf("Reason = %q, want DingAlertFired", ev.Reason)
	}
	if ev.Type != "Warning" {
		t.Errorf("Type = %q, want Warning", ev.Type)
	}
	if ev.Message != "val_loss spike: 12.4 on epoch 7" {
		t.Errorf("Message = %q, want the alert message", ev.Message)
	}
	if ev.InvolvedObject.Kind != "Pod" || ev.InvolvedObject.Name != "ding-pod" {
		t.Errorf("InvolvedObject = %+v, want Pod/ding-pod", ev.InvolvedObject)
	}
	if ev.InvolvedObject.UID != types.UID("pod-uid-1") {
		t.Errorf("InvolvedObject.UID = %q, want pod-uid-1", ev.InvolvedObject.UID)
	}
	if ev.InvolvedObject.Namespace != "ding-test" {
		t.Errorf("InvolvedObject.Namespace = %q, want ding-test", ev.InvolvedObject.Namespace)
	}
	if ev.Source.Component != "ding" {
		t.Errorf("Source.Component = %q, want ding", ev.Source.Component)
	}
	if ev.Source.Host != "node-1" {
		t.Errorf("Source.Host = %q, want node-1", ev.Source.Host)
	}
	if ev.ObjectMeta.GenerateName != "ding-" {
		t.Errorf("GenerateName = %q, want ding-", ev.ObjectMeta.GenerateName)
	}
}

func TestKubernetesEvent_RetriesOnTransientError(t *testing.T) {
	rc := &recordingClient{
		nextErr: func(call int) error {
			if call == 0 {
				return errors.New("connection refused") // transient
			}
			return nil
		},
	}
	n := newTestNotifier(rc)
	defer n.Stop()

	_ = n.Send(sampleAlert())
	if !waitForCalls(rc, 2, 1*time.Second) {
		t.Fatalf("expected 2 Create calls (1 fail + 1 retry success), got %d", rc.callCount())
	}
	// Settle: ensure no third call.
	time.Sleep(50 * time.Millisecond)
	if got := rc.callCount(); got != 2 {
		t.Errorf("expected exactly 2 Create calls, got %d", got)
	}
}

func TestKubernetesEvent_PermanentOnForbidden(t *testing.T) {
	rc := &recordingClient{
		nextErr: func(call int) error {
			return apierrors.NewForbidden(
				schema.GroupResource{Group: "", Resource: "events"},
				"events", errors.New("RBAC denied"))
		},
	}
	n := newTestNotifier(rc)
	defer n.Stop()

	_ = n.Send(sampleAlert())
	if !waitForCalls(rc, 1, 1*time.Second) {
		t.Fatalf("expected 1 Create call, got %d", rc.callCount())
	}
	// Settle: 403 is permanent, so no retries.
	time.Sleep(80 * time.Millisecond)
	if got := rc.callCount(); got != 1 {
		t.Errorf("expected exactly 1 Create call (no retry on Forbidden), got %d", got)
	}
}

func TestKubernetesEvent_DropsAfterMaxAttempts(t *testing.T) {
	rc := &recordingClient{
		nextErr: func(call int) error {
			return errors.New("apiserver unreachable") // transient, every call
		},
	}
	n := newTestNotifier(rc) // maxAttempts=3
	defer n.Stop()

	_ = n.Send(sampleAlert())
	if !waitForCalls(rc, 3, 2*time.Second) {
		t.Fatalf("expected 3 Create calls (max attempts), got %d", rc.callCount())
	}
	// Settle: ensure no fourth call.
	time.Sleep(80 * time.Millisecond)
	if got := rc.callCount(); got != 3 {
		t.Errorf("expected exactly 3 Create calls, got %d", got)
	}
}

func TestKubernetesEvent_Drain_WaitsForInFlight(t *testing.T) {
	releaseCreate := make(chan struct{})
	creating := make(chan struct{}, 1)
	rc := &recordingClient{
		preCall: func() {
			creating <- struct{}{}
			<-releaseCreate
		},
	}
	n := newTestNotifier(rc)

	_ = n.Send(sampleAlert())
	<-creating // worker is mid-Create

	drainDone := make(chan struct{})
	var drainStarted, drainReturned int64
	atomic.StoreInt64(&drainStarted, time.Now().UnixNano())
	go func() {
		n.Drain(2 * time.Second)
		atomic.StoreInt64(&drainReturned, time.Now().UnixNano())
		close(drainDone)
	}()

	// Drain should NOT have returned yet — still waiting on the in-flight Create.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-drainDone:
		t.Fatal("Drain returned before in-flight Create finished")
	default:
	}

	// Release the Create; drain should now return promptly.
	close(releaseCreate)
	select {
	case <-drainDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Drain did not return after Create finished")
	}
}

func TestKubernetesEvent_Drain_RespectsTimeout(t *testing.T) {
	rc := &recordingClient{
		preCall: func() {
			// Block forever — Drain must return when timeout elapses.
			select {}
		},
	}
	n := newTestNotifier(rc)
	_ = n.Send(sampleAlert())

	start := time.Now()
	n.Drain(150 * time.Millisecond)
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("Drain took %v, want ≤500ms (timeout was 150ms)", elapsed)
	}
}

func TestNewKubernetesEventNotifier_OutsideClusterErrors(t *testing.T) {
	// Outside a real K8s cluster, rest.InClusterConfig() fails — the public
	// constructor must surface a clear, scoped error.
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	_, err := NewKubernetesEventNotifier("", "", "", 3, time.Second, nil)
	if err == nil {
		t.Fatal("expected error outside cluster, got nil")
	}
	if !strings.Contains(err.Error(), "in-cluster") {
		t.Errorf("error %q does not mention 'in-cluster' — message should be scoped", err.Error())
	}
}
