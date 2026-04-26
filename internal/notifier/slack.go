package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zuchka/ding/internal/evaluator"
	"github.com/zuchka/ding/internal/metrics"
)

// Block Kit types (Slack API).
type slackTextObj struct {
	Type string `json:"type"` // "plain_text" or "mrkdwn"
	Text string `json:"text"`
}

type slackFieldObj struct {
	Type string `json:"type"` // "mrkdwn"
	Text string `json:"text"`
}

type slackBlock struct {
	Type     string          `json:"type"`
	Text     *slackTextObj   `json:"text,omitempty"`
	Fields   []*slackFieldObj `json:"fields,omitempty"`
	Elements []*slackTextObj  `json:"elements,omitempty"`
}

type slackMessage struct {
	Blocks []slackBlock `json:"blocks"`
}

// SlackNotifier POSTs Slack Block Kit payloads to an incoming webhook URL
// with exponential backoff retry. Run-context fields (branch, commit, run_id,
// exit_code, etc.) are surfaced as structured fields when present in the alert.
type SlackNotifier struct {
	webhookURL     string
	client         *http.Client
	maxAttempts    int
	initialBackoff time.Duration
	queue          chan retryItem // shares retryItem from webhook.go (same package)
	stop           chan struct{}
	stopOnce       sync.Once
	collector      *metrics.Collector // may be nil
}

// NewSlackNotifier creates and starts a SlackNotifier.
// collector may be nil — all instrumentation points guard against nil.
func NewSlackNotifier(url string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *SlackNotifier {
	n := &SlackNotifier{
		webhookURL:     url,
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
func (n *SlackNotifier) Send(alert evaluator.Alert) error {
	msg := buildSlackPayload(alert)
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ding: slack marshal error for rule %q: %v", alert.Rule, err)
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
		log.Printf("ding: slack queue full for rule %q, dropping alert", alert.Rule)
		if n.collector != nil {
			n.collector.IncrWebhookDrop()
		}
	}
	return nil
}

// Stop signals the worker to exit. Safe to call multiple times.
func (n *SlackNotifier) Stop() {
	n.stopOnce.Do(func() { close(n.stop) })
}

func (n *SlackNotifier) worker() {
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
					log.Printf("ding: slack dropped after %d attempts for rule %q: %v", n.maxAttempts, item.rule, err)
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
					log.Printf("ding: slack queue full during retry for rule %q, dropping", item.rule)
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

// deliver performs a single HTTP POST to the Slack webhook URL.
// Returns an error for 5xx or connection errors (retryable).
// Returns nil for 2xx/3xx (success) and 4xx (not retryable — logged and discarded).
func (n *SlackNotifier) deliver(item retryItem) error {
	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(item.payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		log.Printf("ding: slack %s returned %d for rule %q (not retrying)", n.webhookURL, resp.StatusCode, item.rule)
	}
	return nil
}

// buildSlackPayload constructs a Block Kit message from an alert.
// Run-context fields (branch, commit, run_id, etc.) are surfaced as fields
// when present in alert.Labels or alert.Floats — they arrive pre-merged by
// runctx.Apply() before the event reaches the evaluator.
func buildSlackPayload(alert evaluator.Alert) slackMessage {
	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackTextObj{Type: "plain_text", Text: alert.Rule},
		},
	}

	if alert.Message != "" {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackTextObj{Type: "mrkdwn", Text: alert.Message},
		})
	}

	// Build fields: metric/value first, then priority floats (exit_code, duration),
	// then label fields. This ensures the most actionable job-context signals appear
	// even when many labels are present and the 10-field Slack limit is hit.
	fields := []*slackFieldObj{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Metric:* `%s`", alert.Metric)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Value:* `%g`", alert.Value)},
	}

	if ec, ok := alert.Floats["exit_code"]; ok {
		fields = append(fields, &slackFieldObj{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*exit code:* `%d`", int(ec)),
		})
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		fields = append(fields, &slackFieldObj{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*duration:* `%.1fs`", dur),
		})
	}

	// Run-context label fields — order is stable and user-facing.
	for _, key := range []string{"branch", "commit", "repo", "workflow", "job", "actor", "runner", "run_id"} {
		v, ok := alert.Labels[key]
		if !ok || v == "" {
			continue
		}
		display := v
		if key == "commit" && len(v) > 7 {
			display = v[:7]
		}
		label := strings.ReplaceAll(key, "_", " ")
		fields = append(fields, &slackFieldObj{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*%s:* `%s`", label, display),
		})
	}

	// Slack section blocks are limited to 10 fields.
	if len(fields) > 10 {
		fields = fields[:10]
	}

	blocks = append(blocks, slackBlock{
		Type:   "section",
		Fields: fields,
	})

	blocks = append(blocks, slackBlock{
		Type: "context",
		Elements: []*slackTextObj{
			{Type: "mrkdwn", Text: fmt.Sprintf("Fired at %s", alert.FiredAt.Format(time.RFC3339))},
		},
	})

	return slackMessage{Blocks: blocks}
}
