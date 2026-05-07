package notifier

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// eventsClient is the subset of the K8s client we use. Defined as an interface so
// tests can substitute a fake clientset (k8s.io/client-go/kubernetes/fake).
type eventsClient interface {
	Create(ctx context.Context, ns string, event *corev1.Event, opts metav1.CreateOptions) (*corev1.Event, error)
}

// realEventsClient adapts a *kubernetes.Clientset to the eventsClient interface.
type realEventsClient struct{ cs kubernetes.Interface }

func (r realEventsClient) Create(ctx context.Context, ns string, event *corev1.Event, opts metav1.CreateOptions) (*corev1.Event, error) {
	return r.cs.CoreV1().Events(ns).Create(ctx, event, opts)
}

type k8sEventRetryItem struct {
	alert   evaluator.Alert
	attempt int       // number of failed delivery attempts so far (0 = never tried)
	nextAt  time.Time // earliest time to attempt delivery
}

// KubernetesEventNotifier publishes DING alerts as native Kubernetes Events
// (corev1.Event), visible to `kubectl describe pod` and `kubectl get events`.
// Targets the Pod where DING is running (read from POD_NAME / POD_UID via
// downward API). Requires in-cluster ServiceAccount auth; not usable from
// outside a Kubernetes pod.
type KubernetesEventNotifier struct {
	client         eventsClient
	namespace      string
	podName        string
	podUID         types.UID
	nodeName       string
	eventReason    string
	eventType      string
	maxAttempts    int
	initialBackoff time.Duration
	queue          chan k8sEventRetryItem
	stop           chan struct{}
	stopOnce       sync.Once
	collector      *metrics.Collector // may be nil
	// inFlight tracks alerts that have been queued but not yet finalized
	// (delivered, retry-exhausted, or dropped). Drain waits on this so the
	// process doesn't exit while the worker is mid-Create.
	inFlight sync.WaitGroup
}

// NewKubernetesEventNotifier constructs a KubernetesEventNotifier. Reads
// POD_NAME, POD_UID, POD_NAMESPACE, and NODE_NAME from environment (downward
// API). The namespace argument, if non-empty, overrides POD_NAMESPACE;
// otherwise POD_NAMESPACE is required. Defaults: eventReason="DingAlertFired",
// eventType="Warning". Returns an error if not running in-cluster or if
// required downward-API env vars are missing.
func NewKubernetesEventNotifier(namespace, eventReason, eventType string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) (*KubernetesEventNotifier, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("kubernetes_event notifier requires in-cluster ServiceAccount auth: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes_event: build clientset: %w", err)
	}

	podName := os.Getenv("POD_NAME")
	podUID := os.Getenv("POD_UID")
	nodeName := os.Getenv("NODE_NAME")
	ns := namespace
	if ns == "" {
		ns = os.Getenv("POD_NAMESPACE")
	}
	if ns == "" {
		return nil, fmt.Errorf("kubernetes_event notifier requires POD_NAMESPACE env (downward API) or explicit `namespace:` config field")
	}

	if eventReason == "" {
		eventReason = "DingAlertFired"
	}
	if eventType == "" {
		eventType = "Warning"
	}

	return newKubernetesEventNotifierWithClient(realEventsClient{cs: cs}, ns, podName, types.UID(podUID), nodeName, eventReason, eventType, maxAttempts, initialBackoff, collector), nil
}

// newKubernetesEventNotifierWithClient is the test-friendly constructor that
// accepts an injected eventsClient. Production callers should use
// NewKubernetesEventNotifier.
func newKubernetesEventNotifierWithClient(client eventsClient, namespace, podName string, podUID types.UID, nodeName, eventReason, eventType string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *KubernetesEventNotifier {
	n := &KubernetesEventNotifier{
		client:         client,
		namespace:      namespace,
		podName:        podName,
		podUID:         podUID,
		nodeName:       nodeName,
		eventReason:    eventReason,
		eventType:      eventType,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		queue:          make(chan k8sEventRetryItem, 256),
		stop:           make(chan struct{}),
		collector:      collector,
	}
	go n.worker()
	return n
}

// Send enqueues an alert for delivery as a Kubernetes Event. Returns nil always
// (Notifier interface contract). Drops the alert with a logged warning if the
// queue is full.
func (n *KubernetesEventNotifier) Send(alert evaluator.Alert) error {
	item := k8sEventRetryItem{
		alert:   alert,
		attempt: 0,
		nextAt:  time.Now(),
	}
	// Add(1) before the channel send so a fast worker that pulls and finalizes
	// the item is guaranteed to see a positive counter when it calls Done().
	n.inFlight.Add(1)
	select {
	case n.queue <- item:
	default:
		n.inFlight.Done()
		log.Printf("ding: kubernetes_event queue full for rule %q, dropping alert", alert.Rule)
		if n.collector != nil {
			n.collector.IncrWebhookDrop()
		}
	}
	return nil
}

// Stop signals the worker to exit. Safe to call multiple times.
func (n *KubernetesEventNotifier) Stop() {
	n.stopOnce.Do(func() { close(n.stop) })
}

// Drain blocks until all alerts queued before this call have been finalized
// (delivered, retry-exhausted, or dropped) or timeout elapses, then stops the
// worker. Mirrors the slack/webhook contract introduced in PR #6.
func (n *KubernetesEventNotifier) Drain(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		n.inFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		// Hard upper bound: a pathologically slow apiserver can't hang shutdown.
	}
	n.Stop()
}

func (n *KubernetesEventNotifier) worker() {
	for {
		select {
		case <-n.stop:
			return
		case item := <-n.queue:
			delay := time.Until(item.nextAt)
			if delay > 0 {
				t := time.NewTimer(delay)
				select {
				case <-t.C:
				case <-n.stop:
					if !t.Stop() {
						<-t.C
					}
					return
				}
			}
			err := n.deliver(item)
			if err == nil {
				if n.collector != nil {
					n.collector.IncrWebhookSuccess()
				}
				n.inFlight.Done() // delivered — finalize
				continue
			}
			if isPermanentK8sError(err) {
				log.Printf("ding: kubernetes_event permanent failure for rule %q: %v", item.alert.Rule, err)
				if n.collector != nil {
					n.collector.IncrWebhookFailed()
				}
				n.inFlight.Done() // permanent — finalize
				continue
			}
			item.attempt++
			if item.attempt >= n.maxAttempts {
				log.Printf("ding: kubernetes_event dropped after %d attempts for rule %q: %v", n.maxAttempts, item.alert.Rule, err)
				if n.collector != nil {
					n.collector.IncrWebhookFailed()
				}
				n.inFlight.Done() // exhausted retries — finalize
				continue
			}
			backoff := n.initialBackoff * (1 << (item.attempt - 1))
			item.nextAt = time.Now().Add(backoff)
			select {
			case n.queue <- item:
				// Re-enqueued for retry; same logical alert, do NOT Done() yet.
			default:
				log.Printf("ding: kubernetes_event queue full during retry for rule %q, dropping", item.alert.Rule)
				if n.collector != nil {
					n.collector.IncrWebhookDrop()
				}
				n.inFlight.Done() // dropped on retry — finalize
			}
		}
	}
}

// deliver builds a corev1.Event from the alert and creates it via the K8s API.
// Returns an error for retryable failures (network, 5xx, server timeout); the
// caller separately classifies permanent vs retryable via isPermanentK8sError.
func (n *KubernetesEventNotifier) deliver(item k8sEventRetryItem) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := metav1.Now()
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "ding-",
			Namespace:    n.namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       n.podName,
			UID:        n.podUID,
			Namespace:  n.namespace,
		},
		Reason:         n.eventReason,
		Message:        item.alert.Message,
		Type:           n.eventType,
		Source:         corev1.EventSource{Component: "ding", Host: n.nodeName},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	_, err := n.client.Create(ctx, n.namespace, ev, metav1.CreateOptions{})
	return err
}

// isPermanentK8sError reports whether err represents a non-retryable failure
// (4xx-class — RBAC denied, malformed request, validation failure). Network
// errors and 5xx are retryable and return false.
func isPermanentK8sError(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsForbidden(err) ||
		apierrors.IsUnauthorized(err) ||
		apierrors.IsBadRequest(err) ||
		apierrors.IsInvalid(err)
}
