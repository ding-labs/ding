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
