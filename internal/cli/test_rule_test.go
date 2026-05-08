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
