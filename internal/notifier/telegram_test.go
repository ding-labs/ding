package notifier_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/notifier"
)

type telegramPayload struct {
	ChatID    string `json:"chat_id"`
	ParseMode string `json:"parse_mode"`
	Text      string `json:"text"`
}

func parseTelegramPayload(t *testing.T, body []byte) telegramPayload {
	t.Helper()
	var p telegramPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("invalid Telegram JSON: %v\nbody: %s", err, body)
	}
	return p
}

// newTestTelegramNotifier creates a TelegramNotifier pointed at a mock server.
// apiBase should be the httptest server URL (e.g. "http://127.0.0.1:PORT");
// the endpoint becomes apiBase+"/bottest-token/sendMessage", which the mock handles.
func newTestTelegramNotifier(apiBase, chatID string, maxAttempts int, initialBackoff time.Duration) *notifier.TelegramNotifier {
	return notifier.NewTelegramNotifierAt(apiBase, "test-token", chatID, maxAttempts, initialBackoff, nil)
}

func TestTelegramNotifier_Send_success(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := newTestTelegramNotifier(srv.URL, "-1001234567890", 3, 1*time.Millisecond)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		p := parseTelegramPayload(t, body)
		if p.ChatID != "-1001234567890" {
			t.Errorf("chat_id: want -1001234567890, got %q", p.ChatID)
		}
		if p.ParseMode != "HTML" {
			t.Errorf("parse_mode: want HTML, got %q", p.ParseMode)
		}
		if p.Text == "" {
			t.Error("text is empty")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Telegram delivery")
	}
}

func TestBuildTelegramMessage_runContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := newTestTelegramNotifier(srv.URL, "-1001234567890", 3, 1*time.Millisecond)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		p := parseTelegramPayload(t, body)
		text := p.Text

		for _, want := range []string{
			"<b>test_failure</b>",
			"Tests failed on branch main",
			"<b>metric</b>",
			"<code>run.exit</code>",
			"<b>value</b>",
			"<code>1</code>",
			"<b>exit code</b>",
			"<b>duration</b>",
			"<code>42.5s</code>",
			"<b>branch</b>",
			"<code>main</code>",
			"<b>commit</b>",
			"<code>abc1234</code>", // truncated to 7
			"<b>repo</b>",
			"<code>acme/api</code>",
			"<i>2026-04-25T10:00:00Z</i>",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("text missing %q\nfull text:\n%s", want, text)
			}
		}

		// Full commit SHA must not appear (should be truncated).
		if strings.Contains(text, "abc1234def5678") {
			t.Errorf("full commit SHA should be truncated to 7 chars\nfull text:\n%s", text)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Telegram delivery")
	}
}

func TestBuildTelegramMessage_noRunContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := newTestTelegramNotifier(srv.URL, "-1001234567890", 3, 1*time.Millisecond)
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
		p := parseTelegramPayload(t, body)
		text := p.Text

		for _, want := range []string{
			"<b>cpu_spike</b>",
			"<b>metric</b>",
			"<code>cpu_usage</code>",
			"<b>value</b>",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("text missing %q\nfull text:\n%s", want, text)
			}
		}

		// Run-context fields must not appear when absent from the alert.
		for _, absent := range []string{
			"<b>branch</b>", "<b>commit</b>", "<b>repo</b>",
			"<b>exit code</b>", "<b>duration</b>",
		} {
			if strings.Contains(text, absent) {
				t.Errorf("text should not contain %q when absent from alert\nfull text:\n%s", absent, text)
			}
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Telegram delivery")
	}
}

func TestTelegramNotifier_Send_retries5xx(t *testing.T) {
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

	n := newTestTelegramNotifier(srv.URL, "-1001234567890", 5, 1*time.Millisecond)
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

func TestTelegramNotifier_Drain(t *testing.T) {
	delivered := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		select {
		case delivered <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	n := newTestTelegramNotifier(srv.URL, "-1001234567890", 3, 1*time.Millisecond)

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	n.Drain(2 * time.Second)

	select {
	case <-delivered:
		// Delivered before Drain returned.
	default:
		t.Error("alert was not delivered before Drain returned")
	}
}
