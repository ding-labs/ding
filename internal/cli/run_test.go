package cli

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/zuchka/ding/internal/config"
	"github.com/zuchka/ding/internal/evaluator"
	"github.com/zuchka/ding/internal/notifier"
	"github.com/zuchka/ding/internal/runctx"
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

