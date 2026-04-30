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

// Discord embed types.
type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields"`
	Footer      *discordFooter `json:"footer,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

type discordMessage struct {
	Embeds []discordEmbed `json:"embeds"`
}

// DiscordNotifier POSTs Discord embed payloads to an incoming webhook URL
// with exponential backoff retry. Run-context fields (branch, commit, run_id,
// exit_code, etc.) are surfaced as embed fields when present in the alert.
type DiscordNotifier struct {
	webhookURL     string
	client         *http.Client
	maxAttempts    int
	initialBackoff time.Duration
	queue          chan retryItem
	stop           chan struct{}
	stopOnce       sync.Once
	collector      *metrics.Collector // may be nil
}

// NewDiscordNotifier creates and starts a DiscordNotifier.
// collector may be nil — all instrumentation points guard against nil.
func NewDiscordNotifier(url string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *DiscordNotifier {
	n := &DiscordNotifier{
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
func (n *DiscordNotifier) Send(alert evaluator.Alert) error {
	msg := buildDiscordPayload(alert)
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ding: discord marshal error for rule %q: %v", alert.Rule, err)
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
		log.Printf("ding: discord queue full for rule %q, dropping alert", alert.Rule)
		if n.collector != nil {
			n.collector.IncrWebhookDrop()
		}
	}
	return nil
}

// Stop signals the worker to exit. Safe to call multiple times.
func (n *DiscordNotifier) Stop() {
	n.stopOnce.Do(func() { close(n.stop) })
}

// Drain waits up to timeout for all queued alerts to be delivered, then stops
// the worker. Intended for ding run shutdown so in-flight deliveries complete
// before the process exits. Falls back to Stop on timeout.
func (n *DiscordNotifier) Drain(timeout time.Duration) {
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

func (n *DiscordNotifier) worker() {
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
					log.Printf("ding: discord dropped after %d attempts for rule %q: %v", n.maxAttempts, item.rule, err)
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
					log.Printf("ding: discord queue full during retry for rule %q, dropping", item.rule)
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

// deliver performs a single HTTP POST to the Discord webhook URL.
// Returns an error for 5xx or connection errors (retryable).
// Returns nil for 2xx/3xx (success) and 4xx (not retryable — logged and discarded).
func (n *DiscordNotifier) deliver(item retryItem) error {
	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(item.payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		log.Printf("ding: discord %s returned %d for rule %q (not retrying)", n.webhookURL, resp.StatusCode, item.rule)
	}
	return nil
}

// buildDiscordPayload constructs a Discord embed from an alert.
// Run-context fields (branch, commit, run_id, etc.) are surfaced as embed fields
// when present in alert.Labels or alert.Floats — they arrive pre-merged by
// runctx.Apply() before the event reaches the evaluator.
func buildDiscordPayload(alert evaluator.Alert) discordMessage {
	embed := discordEmbed{
		Title: alert.Rule,
		Color: 0xE74C3C, // red
	}

	if alert.Message != "" {
		embed.Description = alert.Message
	}

	// Build fields: metric/value first, then priority floats (exit_code, duration),
	// then label fields. Discord allows up to 25 fields per embed.
	fields := []discordField{
		{Name: "Metric", Value: fmt.Sprintf("`%s`", alert.Metric), Inline: true},
		{Name: "Value", Value: fmt.Sprintf("`%g`", alert.Value), Inline: true},
	}

	if ec, ok := alert.Floats["exit_code"]; ok {
		fields = append(fields, discordField{
			Name:   "exit code",
			Value:  fmt.Sprintf("`%d`", int(ec)),
			Inline: true,
		})
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		fields = append(fields, discordField{
			Name:   "duration",
			Value:  fmt.Sprintf("`%.1fs`", dur),
			Inline: true,
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
		fields = append(fields, discordField{
			Name:   label,
			Value:  fmt.Sprintf("`%s`", display),
			Inline: true,
		})
	}

	embed.Fields = fields
	embed.Footer = &discordFooter{
		Text: fmt.Sprintf("Fired at %s", alert.FiredAt.Format(time.RFC3339)),
	}

	return discordMessage{Embeds: []discordEmbed{embed}}
}
