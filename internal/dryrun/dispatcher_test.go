package dryrun

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/evaluator"
)

func TestLoggingDispatcher_WritesFormattedAlerts(t *testing.T) {
	var buf bytes.Buffer
	d := NewLoggingDispatcher(&TextFormatter{Color: false}, &buf)

	alerts := []evaluator.Alert{
		{Rule: "rule_a", Metric: "m", Value: 1, Message: "msg-a", Notifiers: []string{"slack"}},
		{Rule: "rule_b", Metric: "m", Value: 2, Message: "msg-b", Notifiers: []string{"webhook"}},
	}
	d.Dispatch(alerts)

	out := buf.String()
	if !strings.Contains(out, "rule_a") || !strings.Contains(out, "rule_b") {
		t.Errorf("expected both rule names in output:\n%s", out)
	}
	if !strings.Contains(out, "msg-a") || !strings.Contains(out, "msg-b") {
		t.Errorf("expected both messages in output:\n%s", out)
	}
}

func TestLoggingDispatcher_EmptyAlerts_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	d := NewLoggingDispatcher(&TextFormatter{Color: false}, &buf)
	d.Dispatch(nil)
	d.Dispatch([]evaluator.Alert{})
	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty alerts, got: %q", buf.String())
	}
}
