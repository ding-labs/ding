package notificationstyle

import (
	"testing"

	"github.com/ding-labs/ding/internal/watch"
)

func TestEventStatusUsesLifecycleEvidenceAndExplicitSeverity(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		fields     map[string]any
		want       Status
	}{
		{"incident", "firing", nil, Error},
		{"source failure", "source_error", map[string]any{"severity": "warn"}, Error},
		{"recovery overrides stale error", "recovered", map[string]any{"severity": "error"}, Recovered},
		{"source recovery", "source_recovered", nil, Recovered},
		{"gap", "gap", nil, Warning},
		{"reset", "state_reset", nil, Warning},
		{"warning incident", "firing", map[string]any{"severity": " WARNING "}, Warning},
		{"warning log", "new-event", map[string]any{"level": "WARN"}, Warning},
		{"error log", "new-event", map[string]any{"log.level": "error"}, Error},
		{"fatal log", "changed", map[string]any{"severity": "fatal"}, Error},
		{"explicit severity wins", "new-event", map[string]any{"severity": "warning", "level": "error"}, Warning},
		{"malformed severity", "firing", map[string]any{"severity": map[string]any{"error": true}}, Error},
		{"message is not a classifier", "changed", map[string]any{"message": "no errors, recovered"}, ""},
		{"unknown", "something_recovered", nil, ""},
		{"change", "changed", nil, ""},
		{"event", "new-event", nil, ""},
		{"info is not recovery", "new-event", map[string]any{"severity": "info"}, ""},
		{"test", "test", nil, ""},
		{"resume is not recovery", "resumed", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ForEvent(watch.Event{Type: tc.kind, Fields: tc.fields}); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
