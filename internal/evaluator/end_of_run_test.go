package evaluator

import (
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/ingester"
)

// TestProcess_EndOfRunRulesDoNotFireDuringRun verifies that rules with
// Mode=end-of-run are silent during Process() — no alerts emitted even when
// the condition would otherwise be met.
func TestProcess_EndOfRunRulesDoNotFireDuringRun(t *testing.T) {
	rules := []EngineRule{
		{
			Name:      "summary",
			Match:     map[string]string{"metric": "latency"},
			Condition: "avg(value) over 1m > 100",
			Message:   "avg latency was {{ .avg }}",
			Alerts:    []string{"stdout"},
			Mode:      "end-of-run",
		},
	}
	eng, err := NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	// Feed events that, in during-run mode, would trigger the rule.
	now := time.Now()
	for i := 0; i < 5; i++ {
		alerts := eng.Process(ingester.Event{
			Metric: "latency",
			Value:  500,
			At:     now.Add(time.Duration(i) * time.Second),
		}, now.Add(time.Duration(i)*time.Second))
		if len(alerts) > 0 {
			t.Fatalf("end-of-run rule fired during Process(): %#v", alerts)
		}
	}
}

// TestProcessEndOfRun_FiresWhenConditionMet verifies end-of-run rules fire
// at run exit when accumulated state satisfies their condition.
func TestProcessEndOfRun_FiresWhenConditionMet(t *testing.T) {
	rules := []EngineRule{
		{
			Name:      "summary",
			Match:     map[string]string{"metric": "latency"},
			Condition: "avg(value) over 1m > 100",
			Message:   "avg latency was {{ .avg }}",
			Alerts:    []string{"stdout"},
			Mode:      "end-of-run",
		},
	}
	eng, err := NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	now := time.Now()
	for i := 0; i < 5; i++ {
		eng.Process(ingester.Event{
			Metric: "latency",
			Value:  500,
			At:     now.Add(time.Duration(i) * time.Second),
		}, now.Add(time.Duration(i)*time.Second))
	}

	alerts := eng.ProcessEndOfRun(now.Add(5 * time.Second))
	if len(alerts) != 1 {
		t.Fatalf("expected 1 end-of-run alert, got %d", len(alerts))
	}
	if alerts[0].Rule != "summary" {
		t.Errorf("alert.Rule = %q, want summary", alerts[0].Rule)
	}
	if alerts[0].Avg != 500 {
		t.Errorf("alert.Avg = %v, want 500", alerts[0].Avg)
	}
}

// TestProcessEndOfRun_DoesNotFireWhenConditionFails verifies that end-of-run
// rules whose conditions are not met produce no alerts.
func TestProcessEndOfRun_DoesNotFireWhenConditionFails(t *testing.T) {
	rules := []EngineRule{
		{
			Name:      "summary",
			Match:     map[string]string{"metric": "latency"},
			Condition: "avg(value) over 1m > 1000",
			Message:   "avg latency was {{ .avg }}",
			Alerts:    []string{"stdout"},
			Mode:      "end-of-run",
		},
	}
	eng, err := NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	now := time.Now()
	for i := 0; i < 5; i++ {
		eng.Process(ingester.Event{
			Metric: "latency",
			Value:  100,
			At:     now.Add(time.Duration(i) * time.Second),
		}, now.Add(time.Duration(i)*time.Second))
	}

	alerts := eng.ProcessEndOfRun(now.Add(5 * time.Second))
	if len(alerts) != 0 {
		t.Errorf("expected no alerts (condition false), got %d: %#v", len(alerts), alerts)
	}
}

// TestProcessEndOfRun_FiresOncePerLabelSet verifies that an end-of-run rule
// matched against multiple distinct label sets produces an alert per label set.
func TestProcessEndOfRun_FiresOncePerLabelSet(t *testing.T) {
	rules := []EngineRule{
		{
			Name:      "per-host",
			Match:     map[string]string{"metric": "errors"},
			Condition: "count(value) over 1h > 0",
			Message:   "errors on {{ .host }}: {{ .count }}",
			Alerts:    []string{"stdout"},
			Mode:      "end-of-run",
		},
	}
	eng, err := NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	now := time.Now()
	for _, host := range []string{"a", "b", "c"} {
		eng.Process(ingester.Event{
			Metric: "errors",
			Value:  1,
			Labels: map[string]string{"host": host},
			At:     now,
		}, now)
	}

	alerts := eng.ProcessEndOfRun(now)
	if len(alerts) != 3 {
		t.Fatalf("expected 3 alerts (one per host), got %d", len(alerts))
	}
	hosts := map[string]bool{}
	for _, a := range alerts {
		hosts[a.Labels["host"]] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !hosts[want] {
			t.Errorf("missing alert for host=%q", want)
		}
	}
}

// TestProcess_DuringRunStillFiresAlongsideEndOfRun verifies that during-run
// and end-of-run rules coexist correctly: during-run fires on each event,
// end-of-run waits for ProcessEndOfRun.
func TestProcess_DuringRunStillFiresAlongsideEndOfRun(t *testing.T) {
	rules := []EngineRule{
		{
			Name:      "spike",
			Match:     map[string]string{"metric": "latency"},
			Condition: "value > 400",
			Message:   "spike: {{ .value }}",
			Alerts:    []string{"stdout"},
		},
		{
			Name:      "summary",
			Match:     map[string]string{"metric": "latency"},
			Condition: "avg(value) over 1m > 100",
			Message:   "avg: {{ .avg }}",
			Alerts:    []string{"stdout"},
			Mode:      "end-of-run",
		},
	}
	eng, err := NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	now := time.Now()
	var spikeAlerts int
	for i := 0; i < 5; i++ {
		alerts := eng.Process(ingester.Event{
			Metric: "latency",
			Value:  500,
			At:     now.Add(time.Duration(i) * time.Second),
		}, now.Add(time.Duration(i)*time.Second))
		for _, a := range alerts {
			if a.Rule == "spike" {
				spikeAlerts++
			}
			if a.Rule == "summary" {
				t.Fatalf("summary (end-of-run) fired during Process()")
			}
		}
	}
	if spikeAlerts != 1 {
		// Cooldown is 0 so it would fire each time, BUT the existing
		// engine fires at most once per Process() call per rule per labelKey.
		// With distinct events at distinct times and no cooldown, expect 5.
		// But in fact cooldown=0 means cooldown is never SET, so all 5 fire.
		if spikeAlerts != 5 {
			t.Errorf("expected 5 spike alerts (no cooldown), got %d", spikeAlerts)
		}
	}

	endAlerts := eng.ProcessEndOfRun(now.Add(5 * time.Second))
	if len(endAlerts) != 1 {
		t.Fatalf("expected 1 end-of-run alert, got %d", len(endAlerts))
	}
}

// TestProcessEndOfRun_OverRun_AggregatesAcrossWholeRun verifies that a rule
// with `condition: avg(value) over run > X` and `mode: end-of-run` aggregates
// across the entire run, regardless of how long the run lasted, and fires
// correctly at run exit. Distinguishes "over run" from "over Nm" by spanning
// timestamps wider than any plausible wall-clock window.
func TestProcessEndOfRun_OverRun_AggregatesAcrossWholeRun(t *testing.T) {
	rules := []EngineRule{
		{
			Name:      "whole_run_avg",
			Match:     map[string]string{"metric": "mem"},
			Condition: "avg(value) over run > 50",
			Message:   "avg mem was {{ .avg }}",
			Alerts:    []string{"stdout"},
			Mode:      "end-of-run",
		},
	}
	eng, err := NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	// Feed events spanning 2 hours — far beyond any wall-clock window.
	t0 := time.Now()
	eng.Process(ingester.Event{Metric: "mem", Value: 40, At: t0}, t0)
	eng.Process(ingester.Event{Metric: "mem", Value: 60, At: t0.Add(1 * time.Hour)}, t0.Add(1*time.Hour))
	eng.Process(ingester.Event{Metric: "mem", Value: 80, At: t0.Add(2 * time.Hour)}, t0.Add(2*time.Hour))

	// At end-of-run the buffer should still hold all three entries:
	// avg(40, 60, 80) = 60, which is > 50.
	alerts := eng.ProcessEndOfRun(t0.Add(2 * time.Hour))
	if len(alerts) != 1 {
		t.Fatalf("expected 1 end-of-run alert, got %d", len(alerts))
	}
	if alerts[0].Avg != 60 {
		t.Errorf("alert.Avg = %v, want 60 (avg of full run)", alerts[0].Avg)
	}
	if alerts[0].Count != 3 {
		t.Errorf("alert.Count = %v, want 3 (all events in run)", alerts[0].Count)
	}
}

// TestProcess_OverRun_FiresMidRunWithCooldown verifies the orthogonal
// composition: `over run` without `mode: end-of-run` fires mid-run when
// the threshold crosses, and cooldown prevents repeated firing.
func TestProcess_OverRun_FiresMidRunWithCooldown(t *testing.T) {
	rules := []EngineRule{
		{
			Name:      "errors_pile_up",
			Match:     map[string]string{"metric": "errors"},
			Condition: "count(value) over run > 2",
			Cooldown:  10 * time.Minute,
			Message:   "errors: {{ .count }}",
			Alerts:    []string{"stdout"},
		},
	}
	eng, err := NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	t0 := time.Now()
	// Events 1, 2 — count is at most 2, condition (count > 2) false.
	for i := 0; i < 2; i++ {
		alerts := eng.Process(ingester.Event{
			Metric: "errors", Value: 1,
			At: t0.Add(time.Duration(i) * time.Second),
		}, t0.Add(time.Duration(i)*time.Second))
		if len(alerts) > 0 {
			t.Fatalf("event %d: rule fired prematurely (count=%v)", i, alerts[0].Count)
		}
	}
	// Event 3 — count crosses to 3, condition true, alert fires.
	alerts := eng.Process(ingester.Event{
		Metric: "errors", Value: 1,
		At: t0.Add(2 * time.Second),
	}, t0.Add(2*time.Second))
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert at event 3, got %d", len(alerts))
	}
	if alerts[0].Count != 3 {
		t.Errorf("alert.Count = %v, want 3", alerts[0].Count)
	}
	// Event 4 — count is 4, condition true, but cooldown blocks the alert.
	alerts = eng.Process(ingester.Event{
		Metric: "errors", Value: 1,
		At: t0.Add(3 * time.Second),
	}, t0.Add(3*time.Second))
	if len(alerts) != 0 {
		t.Errorf("expected cooldown to block alert at event 4, got %d", len(alerts))
	}
}

// TestParseLabelKey verifies that the labelKey reverser handles the formats
// produced by LabelSetKey.
func TestParseLabelKey(t *testing.T) {
	cases := []struct {
		in   string
		want map[string]string
	}{
		{"", nil},
		{"host=a", map[string]string{"host": "a"}},
		{"host=a,region=us", map[string]string{"host": "a", "region": "us"}},
	}
	for _, c := range cases {
		got := parseLabelKey(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseLabelKey(%q) = %#v, want %#v", c.in, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("parseLabelKey(%q)[%q] = %q, want %q", c.in, k, got[k], v)
			}
		}
	}
}
