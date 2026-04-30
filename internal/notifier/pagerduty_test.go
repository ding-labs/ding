package notifier_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/notifier"
)

func newTestPagerDutyNotifier(endpoint string, maxAttempts int, initialBackoff time.Duration) *notifier.PagerDutyNotifier {
	return notifier.NewPagerDutyNotifierAt("test-routing-key", maxAttempts, initialBackoff, nil, endpoint)
}

type pdEnvelope struct {
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
	CustomDetails map[string]interface{} `json:"custom_details"`
}

func parsePDPayload(t *testing.T, body []byte) pdEnvelope {
	t.Helper()
	var env pdEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid PagerDuty JSON: %v\nbody: %s", err, body)
	}
	return env
}

func TestPagerDutyNotifier_Send_success(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	n := newTestPagerDutyNotifier(srv.URL, 3, 1*time.Millisecond)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		env := parsePDPayload(t, body)
		if env.RoutingKey != "test-routing-key" {
			t.Errorf("routing_key: want test-routing-key, got %q", env.RoutingKey)
		}
		if env.EventAction != "trigger" {
			t.Errorf("event_action: want trigger, got %q", env.EventAction)
		}
		if env.Payload.Summary == "" {
			t.Error("payload.summary is empty")
		}
		if env.Payload.Severity != "critical" {
			t.Errorf("payload.severity: want critical, got %q", env.Payload.Severity)
		}
		if env.Payload.Timestamp == "" {
			t.Error("payload.timestamp is empty")
		}
		if env.Payload.Source == "" {
			t.Error("payload.source is empty")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for PagerDuty delivery")
	}
}

func TestBuildPagerDutyPayload_customDetails(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	n := newTestPagerDutyNotifier(srv.URL, 3, 1*time.Millisecond)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		env := parsePDPayload(t, body)
		cd := env.Payload.CustomDetails

		for _, key := range []string{"metric", "value", "exit_code", "duration_seconds", "branch", "commit", "repo"} {
			if _, ok := cd[key]; !ok {
				t.Errorf("custom_details missing key %q\nfull payload: %s", key, body)
			}
		}

		// dedup_key should be the rule name
		if env.DedupKey != "test_failure" {
			t.Errorf("dedup_key: want test_failure, got %q", env.DedupKey)
		}

		// source should be repo label
		if env.Payload.Source != "acme/api" {
			t.Errorf("payload.source: want acme/api, got %q", env.Payload.Source)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for PagerDuty delivery")
	}
}

func TestPagerDutyNotifier_Send_retries5xx(t *testing.T) {
	var count atomic.Int32
	delivered := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusAccepted)
			select {
			case delivered <- struct{}{}:
			default:
			}
		}
	}))
	defer srv.Close()

	n := newTestPagerDutyNotifier(srv.URL, 5, 1*time.Millisecond)
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

func TestPagerDutyNotifier_Send_drops4xx(t *testing.T) {
	var count atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	n := newTestPagerDutyNotifier(srv.URL, 3, 1*time.Millisecond)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	// Give the worker time to process
	time.Sleep(200 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Errorf("expected exactly 1 request (no retry on 4xx), got %d", got)
	}
}

func TestPagerDutyNotifier_Drain(t *testing.T) {
	delivered := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		select {
		case delivered <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	n := newTestPagerDutyNotifier(srv.URL, 3, 1*time.Millisecond)

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
