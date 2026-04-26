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

func makeRunAlert() evaluator.Alert {
	return evaluator.Alert{
		Rule:    "test_failure",
		Message: "Tests failed on branch main",
		Metric:  "run.exit",
		Value:   1,
		Labels: map[string]string{
			"branch":  "main",
			"commit":  "abc1234def5678",
			"repo":    "acme/api",
			"runner":  "github-actions",
			"run_id":  "12345",
			"workflow": "ci",
			"job":     "test",
			"actor":   "octocat",
		},
		Floats:  map[string]float64{"exit_code": 1, "duration_seconds": 42.5},
		FiredAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC),
	}
}

func TestSlackNotifier_Send_success(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		body = buf[:n]
		delivered <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewSlackNotifier(srv.URL, 3, 1*time.Millisecond, nil)
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
		if _, ok := msg["blocks"]; !ok {
			t.Errorf("expected 'blocks' key in Slack payload, got: %s", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Slack delivery")
	}
}

func TestSlackNotifier_Send_retries5xx(t *testing.T) {
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

	n := notifier.NewSlackNotifier(srv.URL, 5, 1*time.Millisecond, nil)
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

func TestSlackNotifier_Send_drops4xx(t *testing.T) {
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

	n := notifier.NewSlackNotifier(srv.URL, 3, 1*time.Millisecond, nil)
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

func TestSlackNotifier_Stop(t *testing.T) {
	n := notifier.NewSlackNotifier("http://127.0.0.1:1", 3, 1*time.Millisecond, nil)

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

// slackBlock mirrors the unexported type for test assertions.
type slackBlock struct {
	Type     string           `json:"type"`
	Text     *slackTextObj    `json:"text,omitempty"`
	Fields   []slackFieldObj  `json:"fields,omitempty"`
	Elements []slackTextObj   `json:"elements,omitempty"`
}

type slackTextObj struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackFieldObj struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackMessage struct {
	Blocks []slackBlock `json:"blocks"`
}

func parseSlackPayload(t *testing.T, body []byte) slackMessage {
	t.Helper()
	var msg slackMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("invalid Slack JSON: %v\nbody: %s", err, body)
	}
	return msg
}

func TestBuildSlackPayload_runContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewSlackNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		msg := parseSlackPayload(t, body)

		// Expect: header + section(message) + section(fields) + context = 4 blocks
		if len(msg.Blocks) != 4 {
			t.Errorf("expected 4 blocks, got %d", len(msg.Blocks))
		}

		// Block 0: header with rule name
		if msg.Blocks[0].Type != "header" {
			t.Errorf("block[0] type: want header, got %s", msg.Blocks[0].Type)
		}
		if msg.Blocks[0].Text == nil || msg.Blocks[0].Text.Text != "test_failure" {
			t.Errorf("header text: want test_failure, got %v", msg.Blocks[0].Text)
		}

		// Block 1: section with message
		if msg.Blocks[1].Type != "section" {
			t.Errorf("block[1] type: want section, got %s", msg.Blocks[1].Type)
		}
		if msg.Blocks[1].Text == nil || msg.Blocks[1].Text.Text != "Tests failed on branch main" {
			t.Errorf("message section text: want 'Tests failed on branch main', got %v", msg.Blocks[1].Text)
		}

		// Block 2: section with fields — must include metric, value, and run-context fields
		fieldsBlock := msg.Blocks[2]
		if fieldsBlock.Type != "section" {
			t.Errorf("block[2] type: want section, got %s", fieldsBlock.Type)
		}
		fieldTexts := make(map[string]bool)
		for _, f := range fieldsBlock.Fields {
			fieldTexts[f.Text] = true
		}
		// With all 8 labels + metric + value + exit_code + duration = 12 fields,
		// Slack's 10-field cap trims the tail. Exit_code and duration are prioritized
		// (added before labels), so they always appear.
		for _, want := range []string{
			"*Metric:* `run.exit`",
			"*Value:* `1`",
			"*exit code:* `1`",
			"*duration:* `42.5s`",
			"*branch:* `main`",
			"*commit:* `abc1234`", // truncated to 7
		} {
			if !fieldTexts[want] {
				t.Errorf("expected field %q in fields block; got fields: %v", want, fieldsBlock.Fields)
			}
		}

		// Block 3: context with timestamp
		if msg.Blocks[3].Type != "context" {
			t.Errorf("block[3] type: want context, got %s", msg.Blocks[3].Type)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}

func TestBuildSlackPayload_noRunContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewSlackNotifier(srv.URL, 3, 1*time.Millisecond, nil)
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
		msg := parseSlackPayload(t, body)

		// No message → header + section(fields) + context = 3 blocks
		if len(msg.Blocks) != 3 {
			t.Errorf("expected 3 blocks (no message block), got %d", len(msg.Blocks))
		}

		fieldsBlock := msg.Blocks[1]
		if fieldsBlock.Type != "section" {
			t.Errorf("block[1] type: want section, got %s", fieldsBlock.Type)
		}
		// Only metric and value fields — no run-context
		if len(fieldsBlock.Fields) != 2 {
			t.Errorf("expected 2 fields (metric + value only), got %d: %v", len(fieldsBlock.Fields), fieldsBlock.Fields)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}
