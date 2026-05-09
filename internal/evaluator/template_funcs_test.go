package evaluator

import (
	"testing"
	"time"
)

// TestHumanizeDuration verifies the duration humanization helper renders
// numeric seconds values via Go's native time.Duration.String() format
// (e.g. 30m43s, 4m7.3s, 500ms), and that non-numeric input passes through
// via fmt.Sprint so templates don't crash on bad data.
func TestHumanizeDuration(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{"zero", 0, "0s"},
		{"subsecond_float", 0.5, "500ms"},
		{"int_seconds", 7, "7s"},
		{"float_minutes_with_decimal", 247.3, "4m7.3s"},
		{"int_half_hour_plus", 1843, "30m43s"},
		{"int_two_hours_with_zero_minutes", 7245, "2h0m45s"},
		{"negative_seconds", -30, "-30s"},
		{"explicit_float64", float64(60), "1m0s"},
		{"explicit_int64", int64(60), "1m0s"},
		{"explicit_uint", uint(120), "2m0s"},
		{"non_numeric_string_passthrough", "hello", "hello"},
		{"nil_passthrough", nil, "<nil>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanizeDuration(tt.in)
			if got != tt.want {
				t.Errorf("humanizeDuration(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestDefault verifies the default helper returns the fallback only when
// the value is nil or the empty string. We intentionally do NOT replicate
// sprig's "0/false/empty-collection falls back" behavior — those values
// are real and would create footguns for {{ .exit_code | default 0 }}-
// style rules.
func TestDefault(t *testing.T) {
	tests := []struct {
		name     string
		fallback interface{}
		value    interface{}
		want     interface{}
	}{
		{"nil_value_uses_fallback", "main", nil, "main"},
		{"empty_string_uses_fallback", "main", "", "main"},
		{"non_empty_string_passes_through", "main", "dev", "dev"},
		{"zero_int_passes_through", 99, 0, 0},
		{"false_passes_through", true, false, false},
		{"string_zero_passes_through", "fallback", "0", "0"},
		{"string_false_passes_through", "fallback", "false", "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultValue(tt.fallback, tt.value)
			if got != tt.want {
				t.Errorf("defaultValue(%v, %v) = %v, want %v", tt.fallback, tt.value, got, tt.want)
			}
		})
	}
}

// TestRenderMessage_DefaultHelper_FillsMissingField proves the default
// helper works through the full renderMessage pipeline for a field that
// is not in Labels, Floats, or the alert struct. After missingkey=zero
// is wired, the missing field surfaces as nil in the template scope and
// default returns the fallback.
func TestRenderMessage_DefaultHelper_FillsMissingField(t *testing.T) {
	alert := Alert{
		Rule:    "build_failed",
		Metric:  "run.exit",
		Value:   1,
		Labels:  map[string]string{"runner": "github-actions"},
		FiredAt: time.Now(),
	}

	got := renderMessage(`Build on {{ .branch | default "unknown" }} failed`, alert)

	want := "Build on unknown failed"
	if got != want {
		t.Errorf("renderMessage default helper: got %q, want %q", got, want)
	}
}

// TestRenderMessage_HumanizeDuration_ThroughPipeline proves humanize_duration
// works through the full renderMessage pipeline against the synthetic
// run.exit event shape (duration_seconds in Floats).
func TestRenderMessage_HumanizeDuration_ThroughPipeline(t *testing.T) {
	alert := Alert{
		Rule:    "training_failed",
		Metric:  "run.exit",
		Value:   1,
		Labels:  map[string]string{"runner": "mlflow"},
		Floats:  map[string]float64{"duration_seconds": 1843},
		FiredAt: time.Now(),
	}

	got := renderMessage("Run failed after {{ .duration_seconds | humanize_duration }}", alert)

	want := "Run failed after 30m43s"
	if got != want {
		t.Errorf("renderMessage humanize_duration: got %q, want %q", got, want)
	}
}
