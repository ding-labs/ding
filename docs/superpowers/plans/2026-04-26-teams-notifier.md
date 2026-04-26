# Microsoft Teams Notifier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `teams` notifier type that POSTs Adaptive Card payloads to Microsoft Teams Workflows incoming webhooks, following the existing Slack/Discord pattern exactly.

**Architecture:** `TeamsNotifier` is a queue-backed goroutine worker with exponential backoff retry — identical struct shape and method set to `SlackNotifier` and `DiscordNotifier`. The payload is a Teams Adaptive Card wrapped in the Workflows message envelope (`type: "message"` + `attachments[]`). Config wiring is a one-line `case "teams":` addition in both `config.go` and `server.go`.

**Tech Stack:** Go stdlib (`net/http`, `encoding/json`, `sync`), `httptest` for tests. No new dependencies.

---

## File Map

| File | Action |
|------|--------|
| `internal/notifier/teams.go` | Create — payload types, `buildTeamsPayload`, `TeamsNotifier` |
| `internal/notifier/teams_test.go` | Create — payload shape tests + notifier integration tests |
| `internal/config/config.go` | Modify — add `"teams"` case to `Validate()` |
| `internal/config/config_test.go` | Modify — add two teams validation tests |
| `internal/server/server.go` | Modify — add `"teams"` case to `buildFromConfig()` |
| `ding.yaml.example` | Modify — add Teams comment block |

---

### Task 1: Write the test file (expect compile failure)

**Files:**
- Create: `internal/notifier/teams_test.go`

- [ ] **Step 1: Create teams_test.go**

```go
package notifier_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zuchka/ding/internal/evaluator"
	"github.com/zuchka/ding/internal/notifier"
)

// Mirror unexported Teams types for test assertions.
type teamsTestTextBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Weight   string `json:"weight,omitempty"`
	Size     string `json:"size,omitempty"`
	Wrap     bool   `json:"wrap,omitempty"`
	IsSubtle bool   `json:"isSubtle,omitempty"`
}

type teamsTestFact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

type teamsTestFactSet struct {
	Type  string          `json:"type"`
	Facts []teamsTestFact `json:"facts"`
}

type teamsTestCard struct {
	Schema  string            `json:"$schema"`
	Type    string            `json:"type"`
	Version string            `json:"version"`
	Body    []json.RawMessage `json:"body"`
}

type teamsTestAttachment struct {
	ContentType string        `json:"contentType"`
	ContentURL  *string       `json:"contentUrl"`
	Content     teamsTestCard `json:"content"`
}

type teamsTestMessage struct {
	Type        string                `json:"type"`
	Attachments []teamsTestAttachment `json:"attachments"`
}

func parseTeamsPayload(t *testing.T, body []byte) teamsTestMessage {
	t.Helper()
	var msg teamsTestMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("invalid Teams JSON: %v\nbody: %s", err, body)
	}
	return msg
}

func findFactSet(t *testing.T, body []json.RawMessage) teamsTestFactSet {
	t.Helper()
	for _, raw := range body {
		var el struct {
			Type string `json:"type"`
		}
		json.Unmarshal(raw, &el)
		if el.Type == "FactSet" {
			var fs teamsTestFactSet
			if err := json.Unmarshal(raw, &fs); err != nil {
				t.Fatalf("unmarshal FactSet: %v", err)
			}
			return fs
		}
	}
	t.Fatal("no FactSet found in card body")
	return teamsTestFactSet{}
}

func TestTeamsNotifier_Send_success(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			t.Fatalf("received invalid JSON: %v\nbody: %s", err, body)
		}
		if _, ok := msg["attachments"]; !ok {
			t.Errorf("expected 'attachments' key in Teams payload, got: %s", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Teams delivery")
	}
}

func TestTeamsNotifier_Send_retries5xx(t *testing.T) {
	var count atomic.Int32
	delivered := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			select {
			case delivered <- struct{}{}:
			default:
			}
		}
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 5, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-delivered:
		if got := count.Load(); got != 3 {
			t.Errorf("expected 3 requests (2 failures + 1 success), got %d", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for delivery; got %d requests", count.Load())
	}
}

func TestTeamsNotifier_Send_drops4xx(t *testing.T) {
	var count atomic.Int32
	done := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		select {
		case done <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first request")
	}
	time.Sleep(50 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Errorf("expected exactly 1 POST for 4xx, got %d", got)
	}
}

func TestTeamsNotifier_Stop(t *testing.T) {
	n := notifier.NewTeamsNotifier("http://127.0.0.1:1", 3, 1*time.Millisecond, nil)

	stopped := make(chan struct{})
	go func() {
		n.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() blocked unexpectedly")
	}
}

func TestTeamsNotifier_Drain(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)

	for i := 0; i < 5; i++ {
		_ = n.Send(makeRunAlert())
	}
	n.Drain(2 * time.Second)

	if got := count.Load(); got != 5 {
		t.Errorf("expected 5 deliveries after Drain, got %d", got)
	}
}

func TestBuildTeamsPayload_runContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		msg := parseTeamsPayload(t, body)

		if msg.Type != "message" {
			t.Errorf("message type: want \"message\", got %q", msg.Type)
		}
		if len(msg.Attachments) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
		}
		att := msg.Attachments[0]
		if att.ContentType != "application/vnd.microsoft.card.adaptive" {
			t.Errorf("contentType: want adaptive card, got %q", att.ContentType)
		}
		if att.ContentURL != nil {
			t.Errorf("contentUrl: want nil, got %v", att.ContentURL)
		}

		card := att.Content
		if card.Type != "AdaptiveCard" {
			t.Errorf("card type: want AdaptiveCard, got %q", card.Type)
		}
		if card.Version != "1.5" {
			t.Errorf("card version: want 1.5, got %q", card.Version)
		}

		// First body element: header TextBlock
		var header teamsTestTextBlock
		if err := json.Unmarshal(card.Body[0], &header); err != nil {
			t.Fatalf("unmarshal header: %v", err)
		}
		if header.Text != "test_failure" {
			t.Errorf("header text: want test_failure, got %q", header.Text)
		}
		if header.Weight != "Bolder" {
			t.Errorf("header weight: want Bolder, got %q", header.Weight)
		}
		if header.Wrap {
			t.Error("header TextBlock: Wrap should not be set on header")
		}

		// Second body element: message TextBlock
		var msgBlock teamsTestTextBlock
		if err := json.Unmarshal(card.Body[1], &msgBlock); err != nil {
			t.Fatalf("unmarshal message block: %v", err)
		}
		if msgBlock.Text != "Tests failed on branch main" {
			t.Errorf("message text: want %q, got %q", "Tests failed on branch main", msgBlock.Text)
		}
		if !msgBlock.Wrap {
			t.Error("message TextBlock: want Wrap=true")
		}

		// FactSet: verify key facts
		fs := findFactSet(t, card.Body)
		factMap := make(map[string]string, len(fs.Facts))
		for _, f := range fs.Facts {
			factMap[f.Title] = f.Value
		}
		for title, want := range map[string]string{
			"Metric":    "`run.exit`",
			"Value":     "`1`",
			"exit code": "`1`",
			"duration":  "`42.5s`",
			"branch":    "`main`",
			"commit":    "`abc1234`",
			"repo":      "`acme/api`",
		} {
			if got := factMap[title]; got != want {
				t.Errorf("fact %q: want %q, got %q", title, want, got)
			}
		}

		// Footer: last body element
		var footer teamsTestTextBlock
		if err := json.Unmarshal(card.Body[len(card.Body)-1], &footer); err != nil {
			t.Fatalf("unmarshal footer: %v", err)
		}
		if !footer.IsSubtle {
			t.Error("footer: want IsSubtle=true")
		}
		if footer.Size != "Small" {
			t.Errorf("footer size: want Small, got %q", footer.Size)
		}
		if footer.Text == "" {
			t.Error("footer text is empty")
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}

func TestBuildTeamsPayload_noRunContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	alert := evaluator.Alert{
		Rule:    "cpu_spike",
		Metric:  "cpu_usage",
		Value:   97,
		Labels:  map[string]string{"host": "web-01"},
		FiredAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC),
	}

	if err := n.Send(alert); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		msg := parseTeamsPayload(t, body)

		if len(msg.Attachments) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
		}
		card := msg.Attachments[0].Content

		// No message TextBlock: header + FactSet + footer = 3 elements
		if len(card.Body) != 3 {
			t.Errorf("expected 3 body elements (header + FactSet + footer), got %d", len(card.Body))
		}

		var header teamsTestTextBlock
		json.Unmarshal(card.Body[0], &header)
		if header.Text != "cpu_spike" {
			t.Errorf("header text: want cpu_spike, got %q", header.Text)
		}

		// Only metric and value in FactSet
		fs := findFactSet(t, card.Body)
		if len(fs.Facts) != 2 {
			t.Errorf("expected 2 facts (metric + value only), got %d: %v", len(fs.Facts), fs.Facts)
		}
		if fs.Facts[0].Title != "Metric" {
			t.Errorf("first fact: want Metric, got %q", fs.Facts[0].Title)
		}
		if fs.Facts[1].Title != "Value" {
			t.Errorf("second fact: want Value, got %q", fs.Facts[1].Title)
		}

		var footer teamsTestTextBlock
		json.Unmarshal(card.Body[len(card.Body)-1], &footer)
		if footer.Text == "" {
			t.Error("footer text is empty")
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}
```

- [ ] **Step 2: Run tests — expect compile error**

```bash
cd /Users/zuchka/code/ding && go test ./internal/notifier/... 2>&1 | head -20
```

Expected: `undefined: notifier.NewTeamsNotifier`

---

### Task 2: Implement teams.go

**Files:**
- Create: `internal/notifier/teams.go`

- [ ] **Step 3: Create teams.go**

```go
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
	select {
	case n.queue <- item:
	default:
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

// Drain waits up to timeout for all queued alerts to be delivered, then stops
// the worker. Intended for ding run shutdown so in-flight deliveries complete
// before the process exits.
func (n *TeamsNotifier) Drain(timeout time.Duration) {
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
					continue
				}
				backoff := n.initialBackoff * (1 << (item.attempt - 1))
				item.nextAt = time.Now().Add(backoff)
				select {
				case n.queue <- item:
				default:
					log.Printf("ding: teams queue full during retry for rule %q, dropping", item.rule)
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
```

- [ ] **Step 4: Run notifier tests — expect all PASS**

```bash
cd /Users/zuchka/code/ding && go test ./internal/notifier/... -v -run Teams 2>&1
```

Expected: all `TestTeams*` and `TestBuildTeams*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notifier/teams.go internal/notifier/teams_test.go
git commit -m "feat: Teams notifier with Adaptive Card formatting"
```

---

### Task 3: Config validation

**Files:**
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/config.go`

- [ ] **Step 6: Add failing config tests**

Append to `internal/config/config_test.go`:

```go
func TestValidate_TeamsRetryDefaults(t *testing.T) {
	cfg := &config.Config{
		Notifiers: map[string]config.NotifierConfig{
			"my-teams": {
				Type: "teams",
				URL:  "https://prod-01.westus.logic.azure.com/workflows/test",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nc := cfg.Notifiers["my-teams"]
	if nc.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", nc.MaxAttempts)
	}
	if nc.InitialBackoff.Duration != 1*time.Second {
		t.Errorf("expected InitialBackoff 1s, got %v", nc.InitialBackoff.Duration)
	}
}

func TestValidate_TeamsMissingURL(t *testing.T) {
	cfg := &config.Config{
		Notifiers: map[string]config.NotifierConfig{
			"my-teams": {
				Type: "teams",
				URL:  "",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for teams missing url, got nil")
	}
	if !strings.Contains(err.Error(), "requires a url") {
		t.Errorf("expected error to contain \"requires a url\", got: %v", err)
	}
}
```

- [ ] **Step 7: Run config tests — expect FAIL**

```bash
cd /Users/zuchka/code/ding && go test ./internal/config/... -v -run Teams 2>&1
```

Expected: `TestValidate_TeamsRetryDefaults` FAIL — `unknown type "teams"`

- [ ] **Step 8: Add "teams" case to Validate() in config.go**

In `internal/config/config.go`, inside the `for name, nc := range cfg.Notifiers` switch, add after the `"pagerduty"` case and before `"github_actions"`:

```go
		case "teams":
			if nc.URL == "" {
				return fmt.Errorf("notifier %q: teams type requires a url", name)
			}
			if nc.MaxAttempts == 0 {
				nc.MaxAttempts = 3
			}
			if nc.InitialBackoff.Duration == 0 {
				nc.InitialBackoff.Duration = 1 * time.Second
			}
			cfg.Notifiers[name] = nc
```

- [ ] **Step 9: Run config tests — expect PASS**

```bash
cd /Users/zuchka/code/ding && go test ./internal/config/... -v -run Teams 2>&1
```

Expected: both `TestValidate_Teams*` tests PASS.

- [ ] **Step 10: Run full config test suite to check for regressions**

```bash
cd /Users/zuchka/code/ding && go test ./internal/config/... 2>&1
```

Expected: all PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add teams notifier type to config validation"
```

---

### Task 4: Server wiring

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 12: Add "teams" case to buildFromConfig()**

In `internal/server/server.go`, inside the `for name, nc := range cfg.Notifiers` switch in `buildFromConfig()`, add after the `"pagerduty"` case:

```go
		case "teams":
			notifiers[name] = notifier.NewTeamsNotifier(nc.URL, nc.MaxAttempts, nc.InitialBackoff.Duration, collector)
```

- [ ] **Step 13: Run all tests**

```bash
cd /Users/zuchka/code/ding && go test ./... 2>&1
```

Expected: all PASS.

- [ ] **Step 14: Commit**

```bash
git add internal/server/server.go
git commit -m "feat: wire teams notifier in server buildFromConfig"
```

---

### Task 5: Update ding.yaml.example

**Files:**
- Modify: `ding.yaml.example`

- [ ] **Step 15: Add Teams comment block**

In `ding.yaml.example`, insert the following block after the Discord block (after the `#   initial_backoff: 1s` line for Discord) and before the `#` blank line that precedes the generic webhook block:

```yaml
  #
  # Microsoft Teams (Adaptive Card via Workflows incoming webhook):
  # Get the webhook URL from Teams: channel > Workflows app >
  # "Post to a channel when a webhook request is received".
  # alert-teams:
  #   type: teams
  #   url: https://prod-XX.westus.logic.azure.com:443/workflows/...
  #   max_attempts: 3
  #   initial_backoff: 1s
```

- [ ] **Step 16: Verify the file looks right**

```bash
grep -A 10 "Microsoft Teams" /Users/zuchka/code/ding/ding.yaml.example
```

- [ ] **Step 17: Commit**

```bash
git add ding.yaml.example
git commit -m "docs: add Microsoft Teams notifier example to ding.yaml.example"
```
