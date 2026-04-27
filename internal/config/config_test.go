package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zuchka/ding/internal/config"
)

const validYAML = `
server:
  port: 8080
  format: json
  max_buffer_size: 5000

notifiers:
  alert-slack:
    type: webhook
    url: https://hooks.slack.com/test

rules:
  - name: cpu_spike
    match:
      metric: cpu_usage
    condition: "value > 95"
    cooldown: 1m
    message: "CPU spike: {{ .value }}"
    alert:
      - notifier: alert-slack
      - notifier: stdout
`

func TestLoad_Valid(t *testing.T) {
	f, err := os.CreateTemp("", "ding-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(validYAML)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Format != "json" {
		t.Errorf("expected format json, got %s", cfg.Server.Format)
	}
	if cfg.Server.MaxBufferSize != 5000 {
		t.Errorf("expected max_buffer_size 5000, got %d", cfg.Server.MaxBufferSize)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Rules))
	}
	r := cfg.Rules[0]
	if r.Name != "cpu_spike" {
		t.Errorf("expected rule name cpu_spike, got %s", r.Name)
	}
	if r.Cooldown != time.Minute {
		t.Errorf("expected cooldown 1m, got %v", r.Cooldown)
	}
	if len(r.Alert) != 2 {
		t.Fatalf("expected 2 alert targets, got %d", len(r.Alert))
	}
	if r.Alert[0].Notifier != "alert-slack" {
		t.Errorf("expected alert-slack, got %s", r.Alert[0].Notifier)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/ding.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestValidate_MissingNotifier(t *testing.T) {
	cfg := &config.Config{
		Rules: []config.Rule{
			{
				Name:      "test",
				Condition: "value > 10",
				Alert:     []config.AlertTarget{{Notifier: "nonexistent"}},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing notifier reference")
	}
}

func TestValidate_DefaultPort(t *testing.T) {
	cfg := &config.Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestValidate_DefaultTimeouts(t *testing.T) {
	cfg := &config.Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.ReadTimeout.Duration != 5*time.Second {
		t.Errorf("expected ReadTimeout 5s, got %v", cfg.Server.ReadTimeout.Duration)
	}
	if cfg.Server.WriteTimeout.Duration != 10*time.Second {
		t.Errorf("expected WriteTimeout 10s, got %v", cfg.Server.WriteTimeout.Duration)
	}
	if cfg.Server.IdleTimeout.Duration != 60*time.Second {
		t.Errorf("expected IdleTimeout 60s, got %v", cfg.Server.IdleTimeout.Duration)
	}
}

func TestValidate_DefaultMaxBodyBytes(t *testing.T) {
	cfg := &config.Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.MaxBodyBytes != 1<<20 {
		t.Errorf("expected MaxBodyBytes %d, got %d", 1<<20, cfg.Server.MaxBodyBytes)
	}
}

func TestValidate_PersistenceDefaults(t *testing.T) {
	cfg := &config.Config{
		Persistence: config.PersistenceConfig{
			StateFile: "/tmp/ding.state",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Persistence.FlushInterval.Duration != 30*time.Second {
		t.Errorf("expected FlushInterval 30s, got %v", cfg.Persistence.FlushInterval.Duration)
	}
}

func TestValidate_PersistenceNoStateFile(t *testing.T) {
	cfg := &config.Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Persistence.FlushInterval.Duration != 0 {
		t.Errorf("expected FlushInterval 0 when StateFile empty, got %v", cfg.Persistence.FlushInterval.Duration)
	}
}

func TestValidate_WebhookRetryDefaults(t *testing.T) {
	cfg := &config.Config{
		Notifiers: map[string]config.NotifierConfig{
			"my-webhook": {
				Type: "webhook",
				URL:  "https://example.com/hook",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nc := cfg.Notifiers["my-webhook"]
	if nc.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", nc.MaxAttempts)
	}
	if nc.InitialBackoff.Duration != 1*time.Second {
		t.Errorf("expected InitialBackoff 1s, got %v", nc.InitialBackoff.Duration)
	}
}

func TestValidate_WebhookMissingURL(t *testing.T) {
	cfg := &config.Config{
		Notifiers: map[string]config.NotifierConfig{
			"my-webhook": {
				Type: "webhook",
				URL:  "",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for webhook missing url, got nil")
	}
	if !strings.Contains(err.Error(), "requires a url") {
		t.Errorf("expected error to contain \"requires a url\", got: %v", err)
	}
}

func TestValidate_Guard_MissingURL(t *testing.T) {
	cfg := &config.Config{
		Rules: []config.Rule{
			{
				Name:      "guarded",
				Condition: "value > 10",
				Guard: &config.GuardConfig{
					URL:          "", // missing
					ExpectStatus: 200,
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for guard missing url, got nil")
	}
	if !strings.Contains(err.Error(), "guard.url") {
		t.Errorf("expected error to mention guard.url, got: %v", err)
	}
}

func TestValidate_Guard_MissingExpectStatus(t *testing.T) {
	cfg := &config.Config{
		Rules: []config.Rule{
			{
				Name:      "guarded",
				Condition: "value > 10",
				Guard: &config.GuardConfig{
					URL:          "http://localhost:9000/status",
					ExpectStatus: 0, // missing
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for guard missing expect_status, got nil")
	}
	if !strings.Contains(err.Error(), "guard.expect_status") {
		t.Errorf("expected error to mention guard.expect_status, got: %v", err)
	}
}

func TestValidate_Guard_ValidWithTTLDefault(t *testing.T) {
	cfg := &config.Config{
		Rules: []config.Rule{
			{
				Name:      "guarded",
				Condition: "value > 10",
				Alert:     []config.AlertTarget{{Notifier: "stdout"}},
				Guard: &config.GuardConfig{
					URL:          "http://localhost:9000/status",
					ExpectStatus: 204,
					// TTL intentionally zero — should default to 5s
				},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if cfg.Rules[0].Guard.TTL != 5*time.Second {
		t.Errorf("expected guard TTL default 5s, got %v", cfg.Rules[0].Guard.TTL)
	}
}

func TestValidate_Guard_ValidWithExplicitTTL(t *testing.T) {
	yaml := `
rules:
  - name: guarded
    condition: "value > 10"
    alert:
      - notifier: stdout
    guard:
      url: http://localhost:9000/status
      expect_status: 204
      ttl: 30s
`
	f, err := os.CreateTemp("", "ding-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Rules[0].Guard == nil {
		t.Fatal("expected guard to be non-nil")
	}
	if cfg.Rules[0].Guard.TTL != 30*time.Second {
		t.Errorf("expected guard TTL 30s, got %v", cfg.Rules[0].Guard.TTL)
	}
	if cfg.Rules[0].Guard.ExpectStatus != 204 {
		t.Errorf("expected expect_status 204, got %d", cfg.Rules[0].Guard.ExpectStatus)
	}
}

func TestValidate_TeamsRetryDefaults(t *testing.T) {
	cfg := &config.Config{
		Notifiers: map[string]config.NotifierConfig{
			"my-teams": {
				Type: "teams",
				URL:  "https://prod-01.westus.logic.azure.com/workflows/test",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nc := cfg.Notifiers["my-teams"]
	if nc.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", nc.MaxAttempts)
	}
	if nc.InitialBackoff.Duration != 1*time.Second {
		t.Errorf("expected InitialBackoff 1s, got %v", nc.InitialBackoff.Duration)
	}
}

func TestValidate_TeamsMissingURL(t *testing.T) {
	cfg := &config.Config{
		Notifiers: map[string]config.NotifierConfig{
			"my-teams": {
				Type: "teams",
				URL:  "",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for teams missing url, got nil")
	}
	if !strings.Contains(err.Error(), "requires a url") {
		t.Errorf("expected error to contain \"requires a url\", got: %v", err)
	}
}

func TestLoad_JQField(t *testing.T) {
	yaml := `
server:
  port: 8080
  jq: '.events[] | {metric: .name, value: .v}'
rules:
  - name: test_rule
    condition: "value > 0"
    alert:
      - notifier: stdout
`
	f, err := os.CreateTemp("", "ding-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := ".events[] | {metric: .name, value: .v}"
	if cfg.Server.JQ != want {
		t.Errorf("expected JQ %q, got %q", want, cfg.Server.JQ)
	}
}

func TestLoad_ExpandsNotifierURL(t *testing.T) {
	t.Setenv("T2A_INT_SLACK_URL", "https://hooks.slack.com/services/T1/B2/abc")
	yaml := `
notifiers:
  alert-slack:
    type: slack
    url: ${T2A_INT_SLACK_URL}
rules:
  - name: cpu_spike
    match: { metric: cpu_usage }
    condition: "value > 95"
    alert: [ { notifier: alert-slack } ]
`
	f, err := os.CreateTemp("", "ding-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cfg.Notifiers["alert-slack"].URL; got != "https://hooks.slack.com/services/T1/B2/abc" {
		t.Errorf("expected expanded URL, got %q", got)
	}
}

func TestLoad_UnsetEnvVarErrors(t *testing.T) {
	t.Setenv("T2A_INT_MISSING", "sentinel")
	if err := os.Unsetenv("T2A_INT_MISSING"); err != nil {
		t.Fatal(err)
	}
	yaml := `
notifiers:
  s:
    type: slack
    url: ${T2A_INT_MISSING}
rules:
  - name: r
    condition: "value > 0"
    alert: [ { notifier: s } ]
`
	f, _ := os.CreateTemp("", "ding-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	_, err := config.Load(f.Name())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "T2A_INT_MISSING") {
		t.Errorf("err should name the unset var; got: %v", err)
	}
	if !strings.Contains(err.Error(), "unset env var") {
		t.Errorf("err should mention 'unset env var'; got: %v", err)
	}
}

func TestLoad_MultipleNotifiersOneEnvUnset(t *testing.T) {
	t.Setenv("T2A_INT_SLACK_OK", "https://example.com/slack")
	t.Setenv("T2A_INT_PD_KEY", "sentinel")
	if err := os.Unsetenv("T2A_INT_PD_KEY"); err != nil {
		t.Fatal(err)
	}
	yaml := `
notifiers:
  s:
    type: slack
    url: ${T2A_INT_SLACK_OK}
  pd:
    type: pagerduty
    token: ${T2A_INT_PD_KEY}
rules:
  - name: r
    condition: "value > 0"
    alert: [ { notifier: s } ]
`
	f, _ := os.CreateTemp("", "ding-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	_, err := config.Load(f.Name())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "T2A_INT_PD_KEY") {
		t.Errorf("err should name PD key; got: %v", err)
	}
	if strings.Contains(err.Error(), "T2A_INT_SLACK_OK") {
		t.Errorf("err must not mention SLACK_OK (it was set); got: %v", err)
	}
}

func TestLoad_NoEnvVarsInConfig_StillWorks(t *testing.T) {
	// Backward-compat smoke: existing-style config with no ${VAR} loads exactly
	// as before. Reuses the validYAML fixture from TestLoad_Valid above.
	f, _ := os.CreateTemp("", "ding-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString(validYAML)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_EmptyEnvVarSet(t *testing.T) {
	// Env var explicitly set to empty string is valid; expands to "".
	// Validate() will then reject the slack notifier (URL required), but
	// expansion itself must not error.
	t.Setenv("T2A_INT_EMPTY", "")
	yaml := `
notifiers:
  s:
    type: webhook
    url: https://example.com/${T2A_INT_EMPTY}path
rules:
  - name: r
    condition: "value > 0"
    alert: [ { notifier: s } ]
`
	f, _ := os.CreateTemp("", "ding-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cfg.Notifiers["s"].URL; got != "https://example.com/path" {
		t.Errorf("expected URL with empty expanded, got %q", got)
	}
}
