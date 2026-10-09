package migrate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/transform"
)

const supported = `server: {format: json}
notifiers:
  ops: {type: slack, url: '${OPS_URL}'}
rules:
- name: cpu
  match: {metric: cpu}
  condition: value > 90
  cooldown: 1m
  message: 'CPU {{ .value }} on {{ .host }}'
  alert: [{notifier: ops}]
`

func TestWholeRuleConversionAndSecretBindings(t *testing.T) {
	t.Setenv("OPS_URL", "resolved-secret-that-must-never-appear")
	r, err := Convert([]byte(supported))
	if err != nil || r.Report.Converted != 1 {
		t.Fatal(r.Report, err)
	}
	if r.Report.Bindings[0].Environment != "OPS_URL" {
		t.Fatal(r.Report)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "resolved-secret") {
		t.Fatal("resolved environment")
	}
	bundle, err := plan.Parse(r.Files[0].Content)
	if err != nil {
		t.Fatal(err)
	}
	p := bundle.Watches[0]
	if p.Definition.Spec.Policy.Trigger != "level" || p.Definition.Spec.Policy.Interval != "1m0s" || len(p.Definition.Spec.Destinations[0].Events) != 1 || p.Definition.Spec.Destinations[0].Events[0] != "firing" {
		t.Fatal(p)
	}
	literal := strings.Replace(supported, "${OPS_URL}", "https://example.com/secret-path", 1)
	r, err = Convert([]byte(literal))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(r.Report)
	if strings.Contains(string(raw), "secret-path") || strings.Contains(string(r.Files[0].Content), "secret-path") {
		t.Fatal("copied literal credential")
	}
	if !strings.HasPrefix(r.Report.Bindings[0].Environment, "DING_MIGRATED_") {
		t.Fatal(r.Report)
	}
}
func TestUnsupportedRuleReportsEveryDefectWithoutPartialManifest(t *testing.T) {
	config := supported + `- name: unsupported
  condition: avg(value) over run > 4
  mode: end-of-run
  guard: {url: https://example.com, expect_status: 200}
  message: '{{ .avg }} {{ .value | humanize_duration }}'
  alert: [{notifier: github_actions}, {notifier: ops}]
`
	r, err := Convert([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	if r.Report.Converted != 1 || r.Report.Unsupported != 1 || len(r.Files) != 1 || r.Report.Rules[1].File != "" || len(r.Report.Rules[1].Issues) < 5 {
		t.Fatal(r.Report)
	}
	for _, tc := range []struct{ from, to string }{
		{"format: json", "format: prometheus"}, {"condition: value > 90", "condition: bad"}, {"cooldown: 1m", "cooldown: -1s"}, {"{{ .value }}", "{{ .avg }}"}, {"metric: cpu", "metric: run.exit"}, {"CPU {{ .value }}", "${MESSAGE}"}, {"type: slack", "type: kubernetes_event"}, {"url: '${OPS_URL}'", "url: ''"},
	} {
		r, err := Convert([]byte(strings.Replace(supported, tc.from, tc.to, 1)))
		if err != nil || r.Report.Unsupported != 1 || len(r.Files) != 0 {
			t.Fatal(tc, r.Report, err)
		}
	}
}
func TestMigrationRejectsAmbiguousConfiguration(t *testing.T) {
	for _, raw := range []string{"", "rules: []", "rules: &x []\nnotifiers: *x", supported + "---\nrules: []", supported + "unknown: 1", strings.Replace(supported, "name: cpu", "name: cpu\n  name: duplicate", 1), strings.Repeat("x", 1<<20+1)} {
		if _, err := Convert([]byte(raw)); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
}
func TestProjectionPreservesLabelIdentityAndBatchedJQ(t *testing.T) {
	cfg := strings.Replace(supported, "server: {format: json}", `server: {format: json, jq: '.events'}`, 1)
	r, err := Convert([]byte(cfg))
	if err != nil || len(r.Files) != 1 {
		t.Fatal(r.Report, err)
	}
	bundle, _ := plan.Parse(r.Files[0].Content)
	outputs, err := transform.Project(context.Background(), []byte(`{"events":[{"metric":"cpu","value":91,"host":"a","z":"x","n":1},{"n":2,"z":"x","host":"a","value":92,"metric":"cpu"}]}`), bundle.Watches[0].Definition.Spec.Source.JQ, nil, 100, 1<<20)
	if err != nil || len(outputs) != 2 || outputs[0]["legacy_group"] != outputs[1]["legacy_group"] {
		t.Fatal(outputs, err)
	}
	if _, err := transform.Project(context.Background(), []byte(`{"events":[{"metric":"cpu","value":"bad"}]}`), bundle.Watches[0].Definition.Spec.Source.JQ, nil, 100, 1<<20); err == nil {
		t.Fatal("wrong value type accepted")
	}
}
func TestWriteNeverOverwritesAndRemovesFailedExport(t *testing.T) {
	r, err := Convert([]byte(supported))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "watches")
	if err := Write(out, r); err != nil {
		t.Fatal(err)
	}
	if err := Write(out, r); err == nil {
		t.Fatal("overwrote output")
	}
	if _, err := os.Stat(filepath.Join(out, "report.json")); err != nil {
		t.Fatal(err)
	}
	failed := filepath.Join(t.TempDir(), "failed")
	r.Files = append(r.Files, r.Files[0])
	if err := Write(failed, r); err == nil {
		t.Fatal("duplicate output allowed")
	}
	if _, err := os.Stat(failed); !os.IsNotExist(err) {
		t.Fatal("partial export left", err)
	}
	failed = filepath.Join(t.TempDir(), "escape")
	r.Files = []File{{Name: "../escape.yaml", Content: []byte("x")}}
	if Write(failed, r) == nil {
		t.Fatal("path escape allowed")
	}
}
