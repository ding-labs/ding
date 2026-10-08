package notifier

import (
	"context"
	"fmt"

	"github.com/ding-labs/ding/internal/delivery"
	"os"
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

// KubernetesEventNotifier publishes DING alerts as native Kubernetes Events
// (corev1.Event), visible to `kubectl describe pod` and `kubectl get events`.
// Targets the Pod where DING is running (read from POD_NAME / POD_UID via
// downward API). Requires in-cluster ServiceAccount auth; not usable from
// outside a Kubernetes pod.
type KubernetesEventNotifier struct {
	client      eventsClient
	namespace   string
	podName     string
	podUID      types.UID
	nodeName    string
	eventReason string
	eventType   string
	*queuedNotifier
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
		queuedNotifier: newQueuedNotifier(maxAttempts, initialBackoff, collector),
	}
	return n
}

func (n *KubernetesEventNotifier) Send(alert evaluator.Alert) error {
	message := alert.Message
	return n.enqueue(alert.Rule, func(ctx context.Context) delivery.Result {
		err := n.deliver(ctx, message)
		if err == nil {
			return delivery.Result{Outcome: delivery.Delivered}
		}
		result := delivery.Result{Outcome: delivery.Retryable, Detail: "Kubernetes event delivery failed"}
		if isPermanentK8sError(err) {
			result.Outcome = delivery.Permanent
		}
		if seconds, ok := apierrors.SuggestsClientDelay(err); ok {
			result.RetryAt = time.Now().Add(time.Duration(seconds) * time.Second)
		}
		return result
	})
}
func (n *KubernetesEventNotifier) deliver(ctx context.Context, message string) error {
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
		Message:        message,
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
