package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
)

const pagerDutyEventsV2URL = "https://events.pagerduty.com/v2/enqueue"

// PagerDutyNotifier POSTs events to the PagerDuty Events API v2 with
// exponential backoff retry. The routing key is sent in the JSON body.
type PagerDutyNotifier struct {
	endpoint       string
	routingKey     string
	client         *http.Client
	maxAttempts    int
	initialBackoff time.Duration
	queue          chan retryItem
	stop           chan struct{}
	stopOnce       sync.Once
	collector      *metrics.Collector // may be nil
}

// NewPagerDutyNotifier creates and starts a PagerDutyNotifier using the
// default PagerDuty Events API v2 endpoint.
func NewPagerDutyNotifier(routingKey string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *PagerDutyNotifier {
	return NewPagerDutyNotifierAt(routingKey, maxAttempts, initialBackoff, collector, pagerDutyEventsV2URL)
}

// NewPagerDutyNotifierAt is like NewPagerDutyNotifier but uses a custom
// endpoint. Useful for directing requests to a test server.
func NewPagerDutyNotifierAt(routingKey string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector, endpoint string) *PagerDutyNotifier {
	n := &PagerDutyNotifier{
		endpoint:       endpoint,
		routingKey:     routingKey,
		client:         &http.Client{Timeout: 10 * time.Second},
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		queue:          make(chan retryItem, 256),
		stop:           make(chan struct{}),
		collector:      collector,
	}
	go n.worker()
	return n
}

// Send enqueues an alert for delivery. Returns nil always (Notifier interface contract).
// If the queue is full, the alert is dropped and a warning is logged.
func (n *PagerDutyNotifier) Send(alert evaluator.Alert) error {
	data, err := json.Marshal(buildPagerDutyPayload(alert, n.routingKey))
	if err != nil {
		log.Printf("ding: pagerduty marshal error for rule %q: %v", alert.Rule, err)
		return nil
	}
	item := retryItem{
		payload: data,
		rule:    alert.Rule,
		attempt: 0,
		nextAt:  time.Now(),
	}
	select {
	case n.queue <- item:
	default:
		log.Printf("ding: pagerduty queue full for rule %q, dropping alert", alert.Rule)
		if n.collector != nil {
			n.collector.IncrWebhookDrop()
		}
	}
	return nil
}

// Stop signals the worker to exit. Safe to call multiple times.
func (n *PagerDutyNotifier) Stop() {
	n.stopOnce.Do(func() { close(n.stop) })
}

// Drain waits up to timeout for all queued alerts to be delivered, then stops
// the worker.
func (n *PagerDutyNotifier) Drain(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(n.queue) == 0 {
			time.Sleep(150 * time.Millisecond)
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	n.Stop()
}

func (n *PagerDutyNotifier) worker() {
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
			if err := n.deliver(item); err != nil {
				item.attempt++
				if item.attempt >= n.maxAttempts {
					log.Printf("ding: pagerduty dropped after %d attempts for rule %q: %v", n.maxAttempts, item.rule, err)
					if n.collector != nil {
						n.collector.IncrWebhookFailed()
					}
					continue
				}
				backoff := n.initialBackoff * (1 << (item.attempt - 1))
				item.nextAt = time.Now().Add(backoff)
				select {
				case n.queue <- item:
				default:
					log.Printf("ding: pagerduty queue full during retry for rule %q, dropping", item.rule)
					if n.collector != nil {
						n.collector.IncrWebhookDrop()
					}
				}
			} else {
				if n.collector != nil {
					n.collector.IncrWebhookSuccess()
				}
			}
		}
	}
}

// deliver performs a single HTTP POST to the PagerDuty Events API.
// Returns an error for 5xx or connection errors (retryable).
// Returns nil for 2xx/3xx (success) and 4xx (not retryable — logged and discarded).
func (n *PagerDutyNotifier) deliver(item retryItem) error {
	resp, err := n.client.Post(n.endpoint, "application/json", bytes.NewReader(item.payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("pagerduty api returned %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		log.Printf("ding: pagerduty %s returned %d for rule %q (not retrying)", n.endpoint, resp.StatusCode, item.rule)
	}
	return nil
}

type pdPayload struct {
	RoutingKey  string      `json:"routing_key"`
	EventAction string      `json:"event_action"`
	DedupKey    string      `json:"dedup_key"`
	Payload     pdEventBody `json:"payload"`
}

type pdEventBody struct {
	Summary       string                 `json:"summary"`
	Source        string                 `json:"source"`
	Severity      string                 `json:"severity"`
	Timestamp     string                 `json:"timestamp"`
	CustomDetails map[string]interface{} `json:"custom_details,omitempty"`
}

func buildPagerDutyPayload(alert evaluator.Alert, routingKey string) pdPayload {
	summary := alert.Rule
	if alert.Message != "" {
		summary = alert.Rule + ": " + alert.Message
	}
	if len(summary) > 1024 {
		summary = summary[:1024]
	}

	source := alert.Rule
	if v := alert.Labels["repo"]; v != "" {
		source = v
	} else if v := alert.Labels["workflow"]; v != "" {
		source = v
	}

	cd := make(map[string]interface{})
	cd["metric"] = alert.Metric
	cd["value"] = fmt.Sprintf("%g", alert.Value)
	if ec, ok := alert.Floats["exit_code"]; ok {
		cd["exit_code"] = fmt.Sprintf("%g", ec)
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		cd["duration_seconds"] = fmt.Sprintf("%g", dur)
	}
	keys := make([]string, 0, len(alert.Labels))
	for k := range alert.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cd[k] = alert.Labels[k]
	}

	return pdPayload{
		RoutingKey:  routingKey,
		EventAction: "trigger",
		DedupKey:    alert.Rule,
		Payload: pdEventBody{
			Summary:       summary,
			Source:        source,
			Severity:      "critical",
			Timestamp:     alert.FiredAt.UTC().Format(time.RFC3339),
			CustomDetails: cd,
		},
	}
}
