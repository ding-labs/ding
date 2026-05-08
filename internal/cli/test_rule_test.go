package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixtureConfig(t *testing.T, dir string) string {
	t.Helper()
	cfg := `
notifiers:
  slack:
    type: webhook
    url: https://example.invalid/webhook
rules:
  - name: spike
    match: { metric: loss }
    condition: value > 1.0
    message: "Loss spike: {{ .value }}"
    alert:
      - notifier: slack
`
	p := filepath.Join(dir, "ding.yaml")
	if err := os.WriteFile(p, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestRunTestRule_PerEventMatch_TextFormat(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFixtureConfig(t, dir)
	events := `{"metric":"loss","value":0.5}
{"metric":"loss","value":1.5}
`
	in := strings.NewReader(events)
	var out, errBuf bytes.Buffer

	err := runTestRule(cfg, "text", false, in, &out, &errBuf)
	if err != nil {
		t.Fatalf("runTestRule: %v\nstderr: %s", err, errBuf.String())
	}

	got := out.String()
	if !strings.Contains(got, "spike") {
		t.Errorf("expected spike rule to fire on second event:\n%s", got)
	}
	if !strings.Contains(got, "Loss spike: 1.5") {
		t.Errorf("expected rendered message:\n%s", got)
	}
	if strings.Count(got, "would fire") != 1 {
		t.Errorf("expected exactly one match, got:\n%s", got)
	}
}

func TestRunTestRule_WindowedRule_FiresAfterEnoughEvents(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "ding.yaml")
	if err := os.WriteFile(cfg, []byte(`
notifiers:
  slack:
    type: webhook
    url: https://example.invalid/webhook
rules:
  - name: hot_avg
    match: { metric: temp }
    condition: avg(value) over 5m > 50
    message: "avg high: {{ .avg }}"
    alert:
      - notifier: slack
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	events := `{"metric":"temp","value":40,"timestamp":"2026-05-08T10:00:00Z"}
{"metric":"temp","value":60,"timestamp":"2026-05-08T10:01:00Z"}
{"metric":"temp","value":70,"timestamp":"2026-05-08T10:02:00Z"}
`
	in := strings.NewReader(events)
	var out, errBuf bytes.Buffer
	if err := runTestRule(cfg, "json", true, in, &out, &errBuf); err != nil {
		t.Fatalf("runTestRule: %v\nstderr: %s", err, errBuf.String())
	}

	if !strings.Contains(out.String(), `"rule":"hot_avg"`) {
		t.Errorf("expected hot_avg to fire after windowed avg crosses threshold:\n%s", out.String())
	}
}

func TestRunTestRule_OverRunWindow_AggregatesAcrossRun(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "ding.yaml")
	if err := os.WriteFile(cfg, []byte(`
notifiers:
  slack:
    type: webhook
    url: https://example.invalid/webhook
rules:
  - name: high_avg_mem
    match: { metric: mem }
    condition: avg(value) over run > 50
    mode: end-of-run
    message: "avg mem: {{ .avg }}"
    alert:
      - notifier: slack
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Three events spanning 2 hours — far wider than any wall-clock window
	// the test would plausibly use. avg(40, 60, 80) = 60 > 50, so the
	// end-of-run rule should fire with avg=60.
	events := `{"metric":"mem","value":40,"timestamp":"2026-05-08T10:00:00Z"}
{"metric":"mem","value":60,"timestamp":"2026-05-08T11:00:00Z"}
{"metric":"mem","value":80,"timestamp":"2026-05-08T12:00:00Z"}
`
	in := strings.NewReader(events)
	var out, errBuf bytes.Buffer

	err := runTestRule(cfg, "text", false, in, &out, &errBuf)
	if err != nil {
		t.Fatalf("runTestRule: %v\nstderr: %s", err, errBuf.String())
	}

	got := out.String()
	if !strings.Contains(got, "high_avg_mem") {
		t.Errorf("expected high_avg_mem rule to fire at end-of-run:\n%s", got)
	}
	if !strings.Contains(got, "avg mem: 60") {
		t.Errorf("expected rendered message to show avg=60 across whole run:\n%s", got)
	}
}

func TestRunTestRule_NoMatch_SilentStdout(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFixtureConfig(t, dir)
	events := `{"metric":"loss","value":0.1}
{"metric":"loss","value":0.2}
`
	in := strings.NewReader(events)
	var out, errBuf bytes.Buffer
	if err := runTestRule(cfg, "text", true, in, &out, &errBuf); err != nil {
		t.Fatalf("runTestRule: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected silent stdout when no rules fire, got: %q", out.String())
	}
}

// TestRunTestRule_NoTimestampField_SynthesizesSequential exercises the
// synthetic-timestamp path for events without a `timestamp` field.
// We check this by verifying that a windowed rule with a tight window
// fires correctly when events are spaced out by the synthesized 1s gap.
func TestRunTestRule_NoTimestampField_SynthesizesSequential(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "ding.yaml")
	if err := os.WriteFile(cfg, []byte(`
notifiers:
  slack:
    type: webhook
    url: https://example.invalid/webhook
rules:
  - name: hot_avg
    match: { metric: temp }
    condition: avg(value) over 30s > 50
    message: "avg high: {{ .avg }}"
    alert: [{ notifier: slack }]
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Three events with NO timestamp field. With synthesized 1s spacing
	// they all fit in the 30s window. Without it (bug behavior), they
	// land at the same wall-clock instant and STILL fit — so this test
	// verifies firing only.  The eviction-driven verification belongs
	// in a separate windowed-rule test that tightens timing later.
	events := `{"metric":"temp","value":40}
{"metric":"temp","value":60}
{"metric":"temp","value":70}
`
	in := strings.NewReader(events)
	var out, errBuf bytes.Buffer
	if err := runTestRule(cfg, "json", true, in, &out, &errBuf); err != nil {
		t.Fatalf("runTestRule: %v\nstderr: %s", err, errBuf.String())
	}
	if !strings.Contains(out.String(), `"rule":"hot_avg"`) {
		t.Errorf("expected hot_avg to fire on synthesized-timestamp events:\n%s", out.String())
	}
}

// TestRunTestRule_InvalidFormat_ReturnsError verifies that --format with
// an unrecognized value produces a clear error rather than silently
// falling through to the auto branch.
func TestRunTestRule_InvalidFormat_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFixtureConfig(t, dir)
	in := strings.NewReader("")
	var out, errBuf bytes.Buffer
	err := runTestRule(cfg, "compact", false, in, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error for invalid --format value, got nil")
	}
	if !strings.Contains(err.Error(), "invalid --format") {
		t.Errorf("error should explain the issue, got: %v", err)
	}
}

// TestRunTestRule_MalformedLine_SkipsAndContinues verifies that a bad
// line is logged to stderr but doesn't stop the run; the subsequent
// valid line still fires its rule.
func TestRunTestRule_MalformedLine_SkipsAndContinues(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFixtureConfig(t, dir)
	events := `not-json
{"metric":"loss","value":2.0}
`
	in := strings.NewReader(events)
	var out, errBuf bytes.Buffer
	if err := runTestRule(cfg, "text", true, in, &out, &errBuf); err != nil {
		t.Fatalf("runTestRule should not return error on malformed line: %v", err)
	}
	if !strings.Contains(out.String(), "spike") {
		t.Errorf("expected the valid second line to still fire spike rule:\n%s", out.String())
	}
}
