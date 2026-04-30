package cli

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/config"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/notifier"
	"github.com/ding-labs/ding/internal/runctx"
)

// captureNotifier records every alert it receives for inspection.
type captureNotifier struct {
	mu     sync.Mutex
	alerts []evaluator.Alert
}

func (c *captureNotifier) Send(alert evaluator.Alert) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alerts = append(c.alerts, alert)
	return nil
}

func (c *captureNotifier) snapshot() []evaluator.Alert {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]evaluator.Alert, len(c.alerts))
	copy(out, c.alerts)
	return out
}

// TestIngestStream_JSONEventsTriggerRules verifies that JSON lines parse
// into events, get run-context labels applied, and reach the rule engine.
func TestIngestStream_JSONEventsTriggerRules(t *testing.T) {
	rules := []evaluator.EngineRule{
		{
			Name:      "spike",
			Match:     map[string]string{"metric": "latency"},
			Condition: "value > 100",
			Message:   "spike on {{ .runner }} (run_id={{ .run_id }})",
			Alerts:    []string{"capture"},
		},
	}
	eng, err := evaluator.NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	cap := &captureNotifier{}
	notifierMap := map[string]notifier.Notifier{"capture": cap}

	rc := &runctx.Context{
		RunID:  "r-test",
		Runner: "github-actions",
		Labels: map[string]string{"branch": "main"},
	}

	cfg := &config.Config{
		Server: config.ServerConfig{Format: "auto"},
	}

	input := strings.NewReader(`{"metric":"latency","value":500}
not a json line — mirrored only
{"metric":"latency","value":1}
{"metric":"latency","value":250}
`)
	var mirror bytes.Buffer

	ingestStream(input, &mirror, eng, notifierMap, nil, cfg, nil, rc)

	alerts := cap.snapshot()
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts (value=500, value=250), got %d: %#v", len(alerts), alerts)
	}
	for _, a := range alerts {
		if a.Labels["run_id"] != "r-test" {
			t.Errorf("alert missing run_id label: %#v", a.Labels)
		}
		if a.Labels["runner"] != "github-actions" {
			t.Errorf("alert missing runner label: %#v", a.Labels)
		}
		if a.Labels["branch"] != "main" {
			t.Errorf("alert missing branch label: %#v", a.Labels)
		}
		if !strings.Contains(a.Message, "r-test") {
			t.Errorf("alert message missing run_id: %q", a.Message)
		}
	}

	// Mirror should have received every line, including the non-JSON one.
	if !strings.Contains(mirror.String(), "not a json line") {
		t.Errorf("mirror missing non-JSON line; got: %q", mirror.String())
	}
}

// TestIngestStream_NoJSONNoCrash verifies that purely non-event output is
// mirrored without errors and produces no alerts.
func TestIngestStream_NoJSONNoCrash(t *testing.T) {
	rules := []evaluator.EngineRule{
		{
			Name:      "any",
			Match:     map[string]string{"metric": "latency"},
			Condition: "value > 100",
			Alerts:    []string{"capture"},
		},
	}
	eng, err := evaluator.NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	cap := &captureNotifier{}
	notifierMap := map[string]notifier.Notifier{"capture": cap}
	rc := &runctx.Context{RunID: "r1", Runner: "local"}
	cfg := &config.Config{Server: config.ServerConfig{Format: "auto"}}

	input := strings.NewReader(`PASSED test_a
FAILED test_b
some random shell output here
`)
	var mirror bytes.Buffer
	ingestStream(input, &mirror, eng, notifierMap, nil, cfg, nil, rc)

	if got := cap.snapshot(); len(got) != 0 {
		t.Errorf("expected no alerts on non-event input, got %d: %#v", len(got), got)
	}
	if !strings.Contains(mirror.String(), "PASSED test_a") {
		t.Error("mirror missing first line")
	}
}

// drainerNotifier records Drain/Stop invocations so tests can assert the
// drain path. Implements both interfaces so we can verify the helper picks
// Drain (the graceful path) over Stop when both are available.
type drainerNotifier struct {
	mu          sync.Mutex
	drainedWith []time.Duration
	stopCount   int
}

func (d *drainerNotifier) Send(_ evaluator.Alert) error { return nil }
func (d *drainerNotifier) Drain(timeout time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.drainedWith = append(d.drainedWith, timeout)
}
func (d *drainerNotifier) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopCount++
}

// stopOnlyNotifier only implements Stop (no Drain) — verifies the helper
// falls back to Stop for legacy/simple notifiers.
type stopOnlyNotifier struct {
	mu        sync.Mutex
	stopCount int
}

func (s *stopOnlyNotifier) Send(_ evaluator.Alert) error { return nil }
func (s *stopOnlyNotifier) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCount++
}

// TestDrainNotifiers_PrefersDrainOverStop locks the contract that notifiers
// implementing Drain (Slack, webhook, PagerDuty, Telegram, etc.) get the
// graceful path with the configured timeout. Stop is only used when Drain
// isn't available. Regression guard for the os.Exit-skips-defers bug
// discovered 2026-04-27 — async alerts on non-zero exits would silently
// drop without an explicit drain call before os.Exit.
func TestDrainNotifiers_PrefersDrainOverStop(t *testing.T) {
	d := &drainerNotifier{}
	s := &stopOnlyNotifier{}
	notifiers := map[string]notifier.Notifier{
		"async":  d,
		"legacy": s,
	}

	timeout := 7 * time.Second
	drainNotifiers(notifiers, timeout)

	if len(d.drainedWith) != 1 {
		t.Errorf("expected drainerNotifier to receive 1 Drain call, got %d", len(d.drainedWith))
	}
	if len(d.drainedWith) >= 1 && d.drainedWith[0] != timeout {
		t.Errorf("Drain called with timeout %v, want %v", d.drainedWith[0], timeout)
	}
	if d.stopCount != 0 {
		t.Errorf("Stop should not have been called on a notifier with Drain support, got %d", d.stopCount)
	}
	if s.stopCount != 1 {
		t.Errorf("expected stopOnlyNotifier to receive 1 Stop call, got %d", s.stopCount)
	}
}

// TestDrainNotifiers_HandlesEmpty verifies the helper is safe with an
// empty notifier map (nothing to drain).
func TestDrainNotifiers_HandlesEmpty(t *testing.T) {
	drainNotifiers(map[string]notifier.Notifier{}, time.Second)
	// no panic, no return value to check — the helper just returns
}

