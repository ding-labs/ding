package dryrun

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
)

func TestTextFormatter_PerEventAlert_NoColor(t *testing.T) {
	alert := evaluator.Alert{
		Rule:      "loss_spike",
		Message:   "Loss spiked to 1.20",
		Metric:    "loss",
		Value:     1.2,
		Notifiers: []string{"slack"},
		FiredAt:   time.Date(2026, 5, 8, 10, 2, 0, 0, time.UTC),
	}

	f := &TextFormatter{Color: false}
	got := string(f.Format(alert))

	wantSubstrings := []string{
		"loss_spike",
		"would fire",
		"slack",
		"metric: loss",
		"value: 1.20",
		"Loss spiked to 1.20",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("TextFormatter output missing %q\noutput: %s", want, got)
		}
	}
	// No-color: no ANSI escape sequences
	if strings.Contains(got, "\x1b[") {
		t.Errorf("TextFormatter Color=false leaked ANSI escapes:\n%s", got)
	}
}

func TestTextFormatter_Color_EmitsANSIEscapes(t *testing.T) {
	alert := evaluator.Alert{
		Rule:      "x",
		Message:   "m",
		Metric:    "loss",
		Value:     1,
		Notifiers: []string{"slack"},
	}
	f := &TextFormatter{Color: true}
	got := string(f.Format(alert))
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("Color=true did not emit ANSI escapes:\n%q", got)
	}
	if !strings.Contains(got, ansiReset) {
		t.Errorf("Color=true output missing reset code — terminal would be left dirty:\n%q", got)
	}
}

func TestTextFormatter_NoNotifiers(t *testing.T) {
	alert := evaluator.Alert{
		Rule:    "x",
		Message: "m",
		Metric:  "loss",
		Value:   1,
		// Notifiers empty
	}
	f := &TextFormatter{Color: false}
	got := string(f.Format(alert))
	if !strings.Contains(got, "(none)") {
		t.Errorf("expected '(none)' for empty Notifiers, got:\n%s", got)
	}
}

func TestJSONFormatter_RoundTrips(t *testing.T) {
	alert := evaluator.Alert{
		Rule:      "loss_spike",
		Message:   "Loss spiked",
		Metric:    "loss",
		Value:     1.2,
		Labels:    map[string]string{"run_id": "abc123"},
		Floats:    map[string]float64{"step": 100},
		Notifiers: []string{"slack", "pagerduty"},
		FiredAt:   time.Date(2026, 5, 8, 10, 2, 0, 0, time.UTC),
		Avg:       1.1,
	}

	f := &JSONFormatter{}
	out := f.Format(alert)

	if !strings.HasSuffix(string(out), "\n") {
		t.Errorf("JSONFormatter output must end with newline (JSONL): %q", string(out))
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("JSONFormatter output is not valid JSON: %v\noutput: %s", err, out)
	}

	wantKeys := []string{"rule", "metric", "value", "message", "alerts", "fired_at", "labels", "floats", "avg"}
	for _, k := range wantKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("JSONFormatter output missing key %q", k)
		}
	}
	if got["rule"] != "loss_spike" {
		t.Errorf("rule mismatch: got %v", got["rule"])
	}
	if alerts, _ := got["alerts"].([]any); len(alerts) != 2 {
		t.Errorf("alerts should be 2 entries: got %v", alerts)
	}
}
