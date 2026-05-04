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

// TelegramNotifier POSTs HTML-formatted messages to a Telegram chat via the
// Bot API with exponential backoff retry. Run-context fields (branch, commit,
// exit_code, etc.) are surfaced as formatted lines when present in the alert.
type TelegramNotifier struct {
	endpoint       string
	chatID         string
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

// NewTelegramNotifier creates and starts a TelegramNotifier.
// token is the Telegram Bot API token; chatID is the destination chat.
// collector may be nil — all instrumentation points guard against nil.
func NewTelegramNotifier(token, chatID string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *TelegramNotifier {
	return NewTelegramNotifierAt("https://api.telegram.org", token, chatID, maxAttempts, initialBackoff, collector)
}

// NewTelegramNotifierAt is like NewTelegramNotifier but uses apiBase instead of
// the default https://api.telegram.org. Useful for directing requests to a
// test server.
func NewTelegramNotifierAt(apiBase, token, chatID string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *TelegramNotifier {
	n := &TelegramNotifier{
		endpoint:       apiBase + "/bot" + token + "/sendMessage",
		chatID:         chatID,
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
func (n *TelegramNotifier) Send(alert evaluator.Alert) error {
	text := buildTelegramMessage(alert)
	body := map[string]string{
		"chat_id":    n.chatID,
		"parse_mode": "HTML",
		"text":       text,
	}
	data, err := json.Marshal(body)
	if err != nil {
		log.Printf("ding: telegram marshal error for rule %q: %v", alert.Rule, err)
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
		log.Printf("ding: telegram queue full for rule %q, dropping alert", alert.Rule)
		if n.collector != nil {
			n.collector.IncrWebhookDrop()
		}
	}
	return nil
}

// Stop signals the worker to exit. Safe to call multiple times.
func (n *TelegramNotifier) Stop() {
	n.stopOnce.Do(func() { close(n.stop) })
}

// Drain blocks until all alerts queued before this call have been finalized
// (delivered, retry-exhausted, or dropped) or timeout elapses, then stops the
// worker. Intended for ding run shutdown so in-flight HTTP POSTs complete
// before the process exits. Falls back to Stop on timeout.
func (n *TelegramNotifier) Drain(timeout time.Duration) {
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

func (n *TelegramNotifier) worker() {
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
					log.Printf("ding: telegram dropped after %d attempts for rule %q: %v", n.maxAttempts, item.rule, err)
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
					log.Printf("ding: telegram queue full during retry for rule %q, dropping", item.rule)
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

// deliver performs a single HTTP POST to the Telegram Bot API.
// Returns an error for 5xx or connection errors (retryable).
// Returns nil for 2xx/3xx (success) and 4xx (not retryable — logged and discarded).
func (n *TelegramNotifier) deliver(item retryItem) error {
	resp, err := n.client.Post(n.endpoint, "application/json", bytes.NewReader(item.payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("telegram api returned %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		log.Printf("ding: telegram %s returned %d for rule %q (not retrying)", n.endpoint, resp.StatusCode, item.rule)
	}
	return nil
}

// buildTelegramMessage constructs an HTML-formatted message string for the
// Telegram Bot API sendMessage endpoint.
func buildTelegramMessage(alert evaluator.Alert) string {
	var b strings.Builder

	fmt.Fprintf(&b, "<b>%s</b>", alert.Rule)

	if alert.Message != "" {
		fmt.Fprintf(&b, "\n\n%s", alert.Message)
	}

	b.WriteString("\n\n")
	fmt.Fprintf(&b, "<b>metric</b>  <code>%s</code>\n", alert.Metric)
	fmt.Fprintf(&b, "<b>value</b>  <code>%g</code>", alert.Value)

	if ec, ok := alert.Floats["exit_code"]; ok {
		fmt.Fprintf(&b, "\n<b>exit code</b>  <code>%d</code>", int(ec))
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		fmt.Fprintf(&b, "\n<b>duration</b>  <code>%.1fs</code>", dur)
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
		fmt.Fprintf(&b, "\n<b>%s</b>  <code>%s</code>", label, display)
	}

	fmt.Fprintf(&b, "\n\n<i>%s</i>", alert.FiredAt.Format(time.RFC3339))

	return b.String()
}
