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

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
)

// Teams Adaptive Card types.
type teamsTextBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Weight   string `json:"weight,omitempty"`
	Size     string `json:"size,omitempty"`
	Wrap     bool   `json:"wrap,omitempty"`
	IsSubtle bool   `json:"isSubtle,omitempty"`
}

type teamsFact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

type teamsFactSet struct {
	Type  string      `json:"type"`
	Facts []teamsFact `json:"facts"`
}

type teamsCard struct {
	Schema  string        `json:"$schema"`
	Type    string        `json:"type"`
	Version string        `json:"version"`
	Body    []interface{} `json:"body"`
}

type teamsAttachment struct {
	ContentType string    `json:"contentType"`
	ContentURL  *string   `json:"contentUrl"`
	Content     teamsCard `json:"content"`
}

type teamsMessage struct {
	Type        string            `json:"type"`
	Attachments []teamsAttachment `json:"attachments"`
}

// TeamsNotifier POSTs Adaptive Card payloads to a Microsoft Teams Workflows
// incoming webhook URL with exponential backoff retry. Run-context fields
// (branch, commit, run_id, exit_code, etc.) are surfaced as FactSet facts
// when present in the alert.
type TeamsNotifier struct {
	webhookURL     string
	client         *http.Client
	maxAttempts    int
	initialBackoff time.Duration
	queue          chan retryItem
	stop           chan struct{}
	stopOnce       sync.Once
	collector      *metrics.Collector // may be nil
	// inFlight tracks alerts that have been queued but not yet finalized
	// (delivered, retry-exhausted, or dropped). Drain waits on this so the
	// process doesn't exit while the worker is mid-POST.
	inFlight sync.WaitGroup
}

// NewTeamsNotifier creates and starts a TeamsNotifier.
// collector may be nil — all instrumentation points guard against nil.
func NewTeamsNotifier(url string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *TeamsNotifier {
	n := &TeamsNotifier{
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
func (n *TeamsNotifier) Send(alert evaluator.Alert) error {
	msg := buildTeamsPayload(alert)
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ding: teams marshal error for rule %q: %v", alert.Rule, err)
		return nil
	}
	item := retryItem{
		payload: data,
		rule:    alert.Rule,
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
		log.Printf("ding: teams queue full for rule %q, dropping alert", alert.Rule)
		if n.collector != nil {
			n.collector.IncrWebhookDrop()
		}
	}
	return nil
}

// Stop signals the worker to exit. Safe to call multiple times.
func (n *TeamsNotifier) Stop() {
	n.stopOnce.Do(func() { close(n.stop) })
}

// Drain blocks until all alerts queued before this call have been finalized
// (delivered, retry-exhausted, or dropped) or timeout elapses, then stops the
// worker. Intended for ding run shutdown so in-flight HTTP POSTs complete
// before the process exits. Falls back to Stop on timeout.
func (n *TeamsNotifier) Drain(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		n.inFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		// Hard upper bound: a pathologically slow notifier can't hang shutdown.
	}
	n.Stop()
}

func (n *TeamsNotifier) worker() {
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
					log.Printf("ding: teams dropped after %d attempts for rule %q: %v", n.maxAttempts, item.rule, err)
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
					log.Printf("ding: teams queue full during retry for rule %q, dropping", item.rule)
					if n.collector != nil {
						n.collector.IncrWebhookDrop()
					}
					n.inFlight.Done() // dropped on retry — finalize
				}
			} else {
				if n.collector != nil {
					n.collector.IncrWebhookSuccess()
				}
				n.inFlight.Done() // delivered — finalize
			}
		}
	}
}

// deliver performs a single HTTP POST to the Teams webhook URL.
// Returns an error for 5xx or connection errors (retryable).
// Returns nil for 2xx/3xx (success) and 4xx (not retryable — logged and discarded).
func (n *TeamsNotifier) deliver(item retryItem) error {
	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(item.payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("teams webhook returned %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		log.Printf("ding: teams %s returned %d for rule %q (not retrying)", n.webhookURL, resp.StatusCode, item.rule)
	}
	return nil
}

// buildTeamsPayload constructs a Teams Adaptive Card message from an alert.
// Run-context fields (branch, commit, run_id, etc.) are surfaced as FactSet facts
// when present in alert.Labels or alert.Floats — they arrive pre-merged by
// runctx.Apply() before the event reaches the evaluator.
func buildTeamsPayload(alert evaluator.Alert) teamsMessage {
	body := []interface{}{
		teamsTextBlock{
			Type:   "TextBlock",
			Text:   alert.Rule,
			Weight: "Bolder",
			Size:   "Medium",
		},
	}

	if alert.Message != "" {
		body = append(body, teamsTextBlock{
			Type: "TextBlock",
			Text: alert.Message,
			Wrap: true,
		})
	}

	facts := []teamsFact{
		{Title: "Metric", Value: fmt.Sprintf("`%s`", alert.Metric)},
		{Title: "Value", Value: fmt.Sprintf("`%g`", alert.Value)},
	}

	if ec, ok := alert.Floats["exit_code"]; ok {
		facts = append(facts, teamsFact{
			Title: "exit code",
			Value: fmt.Sprintf("`%d`", int(ec)),
		})
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		facts = append(facts, teamsFact{
			Title: "duration",
			Value: fmt.Sprintf("`%.1fs`", dur),
		})
	}

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
		facts = append(facts, teamsFact{
			Title: label,
			Value: fmt.Sprintf("`%s`", display),
		})
	}

	body = append(body, teamsFactSet{
		Type:  "FactSet",
		Facts: facts,
	})

	body = append(body, teamsTextBlock{
		Type:     "TextBlock",
		Text:     fmt.Sprintf("Fired at %s", alert.FiredAt.Format(time.RFC3339)),
		IsSubtle: true,
		Size:     "Small",
	})

	card := teamsCard{
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Type:    "AdaptiveCard",
		Version: "1.5",
		Body:    body,
	}

	return teamsMessage{
		Type: "message",
		Attachments: []teamsAttachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				ContentURL:  nil,
				Content:     card,
			},
		},
	}
}
