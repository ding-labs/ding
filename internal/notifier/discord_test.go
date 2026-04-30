package notifier_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/notifier"
)

// discordTestField mirrors the unexported type for test assertions.
type discordTestField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordTestEmbed struct {
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Color       int                `json:"color"`
	Fields      []discordTestField `json:"fields"`
	Footer      *discordTestFooter `json:"footer,omitempty"`
}

type discordTestFooter struct {
	Text string `json:"text"`
}

type discordTestMessage struct {
	Embeds []discordTestEmbed `json:"embeds"`
}

func parseDiscordPayload(t *testing.T, body []byte) discordTestMessage {
	t.Helper()
	var msg discordTestMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("invalid Discord JSON: %v\nbody: %s", err, body)
	}
	return msg
}

func TestDiscordNotifier_Send_success(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewDiscordNotifier(srv.URL, 3, 1*time.Millisecond, nil)
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
		if _, ok := msg["embeds"]; !ok {
			t.Errorf("expected 'embeds' key in Discord payload, got: %s", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Discord delivery")
	}
}

func TestDiscordNotifier_Send_retries5xx(t *testing.T) {
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

	n := notifier.NewDiscordNotifier(srv.URL, 5, 1*time.Millisecond, nil)
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

func TestDiscordNotifier_Send_drops4xx(t *testing.T) {
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

	n := notifier.NewDiscordNotifier(srv.URL, 3, 1*time.Millisecond, nil)
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

func TestDiscordNotifier_Stop(t *testing.T) {
	n := notifier.NewDiscordNotifier("http://127.0.0.1:1", 3, 1*time.Millisecond, nil)

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

func TestBuildDiscordPayload_runContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewDiscordNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		msg := parseDiscordPayload(t, body)

		if len(msg.Embeds) != 1 {
			t.Fatalf("expected 1 embed, got %d", len(msg.Embeds))
		}
		embed := msg.Embeds[0]

		if embed.Title != "test_failure" {
			t.Errorf("embed title: want test_failure, got %q", embed.Title)
		}
		if embed.Description != "Tests failed on branch main" {
			t.Errorf("embed description: want 'Tests failed on branch main', got %q", embed.Description)
		}
		if embed.Footer == nil {
			t.Fatal("embed footer is nil")
		}
		if embed.Footer.Text == "" {
			t.Error("embed footer text is empty")
		}

		// Build a map of field name → value for easy lookup.
		fieldMap := make(map[string]string, len(embed.Fields))
		for _, f := range embed.Fields {
			fieldMap[f.Name] = f.Value
		}

		// Verify priority fields and key run-context fields.
		for name, wantVal := range map[string]string{
			"Metric":    "`run.exit`",
			"Value":     "`1`",
			"exit code": "`1`",
			"duration":  "`42.5s`",
			"branch":    "`main`",
			"commit":    "`abc1234`", // truncated to 7
			"repo":      "`acme/api`",
		} {
			if got := fieldMap[name]; got != wantVal {
				t.Errorf("field %q: want %q, got %q", name, wantVal, got)
			}
		}

		// All fields should be inline.
		for _, f := range embed.Fields {
			if !f.Inline {
				t.Errorf("field %q: expected Inline=true", f.Name)
			}
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}

func TestBuildDiscordPayload_noRunContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewDiscordNotifier(srv.URL, 3, 1*time.Millisecond, nil)
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
		msg := parseDiscordPayload(t, body)

		if len(msg.Embeds) != 1 {
			t.Fatalf("expected 1 embed, got %d", len(msg.Embeds))
		}
		embed := msg.Embeds[0]

		if embed.Title != "cpu_spike" {
			t.Errorf("embed title: want cpu_spike, got %q", embed.Title)
		}
		if embed.Description != "" {
			t.Errorf("embed description: want empty, got %q", embed.Description)
		}
		// Only metric and value — no run-context
		if len(embed.Fields) != 2 {
			t.Errorf("expected 2 fields (metric + value only), got %d: %v", len(embed.Fields), embed.Fields)
		}
		if embed.Footer == nil || embed.Footer.Text == "" {
			t.Error("embed footer missing")
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}
