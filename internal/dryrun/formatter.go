// Package dryrun provides alert formatters and a Dispatcher implementation
// for dry-run modes (ding test-rule and ding run --dry-run). LoggingDispatcher
// satisfies the cli.Dispatcher interface without sending to real notifiers.
package dryrun

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
)

// Formatter renders one Alert as bytes ready for an io.Writer.
type Formatter interface {
	Format(alert evaluator.Alert) []byte
}

// ANSI color codes. Centralized so the table is easy to audit.
const (
	ansiReset = "\x1b[0m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"
	ansiDim   = "\x1b[2m"
)

// TextFormatter renders alerts as multi-line human-readable text.
// Color toggles ANSI escape sequences (off when not on a TTY or with --no-color).
type TextFormatter struct {
	Color bool
}

func (f *TextFormatter) Format(alert evaluator.Alert) []byte {
	var b strings.Builder
	rule := alert.Rule
	notifiers := strings.Join(alert.Notifiers, ", ")
	if notifiers == "" {
		notifiers = "(none)"
	}

	if f.Color {
		fmt.Fprintf(&b, "%s✓ %s%s — %swould fire%s (alerts: %s)\n",
			ansiGreen, rule, ansiReset, ansiCyan, ansiReset, notifiers)
		fmt.Fprintf(&b, "%s  metric: %s · value: %.2f%s\n",
			ansiDim, alert.Metric, alert.Value, ansiReset)
		fmt.Fprintf(&b, "%s  message:%s %s\n", ansiDim, ansiReset, alert.Message)
	} else {
		fmt.Fprintf(&b, "✓ %s — would fire (alerts: %s)\n", rule, notifiers)
		fmt.Fprintf(&b, "  metric: %s · value: %.2f\n", alert.Metric, alert.Value)
		fmt.Fprintf(&b, "  message: %s\n", alert.Message)
	}
	return []byte(b.String())
}

// JSONFormatter renders alerts as one JSON object per line (JSONL).
// Stable schema; safe to pipe to jq.
type JSONFormatter struct{}

type jsonAlertEnvelope struct {
	Rule    string             `json:"rule"`
	Metric  string             `json:"metric"`
	Value   float64            `json:"value"`
	Message string             `json:"message"`
	Alerts  []string           `json:"alerts"`
	Labels  map[string]string  `json:"labels,omitempty"`
	Floats  map[string]float64 `json:"floats,omitempty"`
	FiredAt string             `json:"fired_at"`
	// Aggregates (always present; zero when not windowed — easier for jq)
	Avg   float64 `json:"avg"`
	Max   float64 `json:"max"`
	Min   float64 `json:"min"`
	Count float64 `json:"count"`
	Sum   float64 `json:"sum"`
}

func (f *JSONFormatter) Format(alert evaluator.Alert) []byte {
	env := jsonAlertEnvelope{
		Rule:    alert.Rule,
		Metric:  alert.Metric,
		Value:   alert.Value,
		Message: alert.Message,
		Alerts:  alert.Notifiers,
		Labels:  alert.Labels,
		Floats:  alert.Floats,
		FiredAt: alert.FiredAt.UTC().Format(time.RFC3339Nano),
		Avg:     alert.Avg,
		Max:     alert.Max,
		Min:     alert.Min,
		Count:   alert.Count,
		Sum:     alert.Sum,
	}
	if env.Alerts == nil {
		env.Alerts = []string{}
	}
	// jsonAlertEnvelope contains only stdlib-marshalable types (string, float64,
	// []string, map[string]string, map[string]float64). Marshal cannot fail here.
	b, _ := json.Marshal(env)
	return append(b, '\n')
}
