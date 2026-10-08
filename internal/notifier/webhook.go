package notifier

import (
	"context"
	"encoding/json"
	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
	"net/http"
	"time"
)

type httpNotifier struct {
	*queuedNotifier
	url, provider string
	client        *http.Client
}

func newHTTPNotifier(url, provider string, attempts int, backoff time.Duration, collector *metrics.Collector) *httpNotifier {
	return &httpNotifier{queuedNotifier: newQueuedNotifier(attempts, backoff, collector), url: url, provider: provider, client: &http.Client{Timeout: 10 * time.Second}}
}
func (n *httpNotifier) send(rule string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return n.enqueue(rule, func(ctx context.Context) delivery.Result {
		return delivery.HTTP(ctx, n.client, delivery.Request{URL: n.url, Body: body, Provider: n.provider}, time.Now())
	})
}

type WebhookNotifier struct{ *httpNotifier }

func NewWebhookNotifier(url string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *WebhookNotifier {
	return &WebhookNotifier{httpNotifier: newHTTPNotifier(url, "webhook", maxAttempts, initialBackoff, collector)}
}
func (n *WebhookNotifier) Send(alert evaluator.Alert) error {
	return n.send(alert.Rule, buildPayload(alert))
}
