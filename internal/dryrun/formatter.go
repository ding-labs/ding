// Package dryrun provides alert formatters and a Dispatcher implementation
// for dry-run modes (ding test-rule and ding run --dry-run). LoggingDispatcher
// satisfies the cli.Dispatcher interface without sending to real notifiers.
package dryrun

import (
	"fmt"
	"strings"

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
		fmt.Fprintf(&b, "  message: %s\n", alert.Message)
	} else {
		fmt.Fprintf(&b, "✓ %s — would fire (alerts: %s)\n", rule, notifiers)
		fmt.Fprintf(&b, "  metric: %s · value: %.2f\n", alert.Metric, alert.Value)
		fmt.Fprintf(&b, "  message: %s\n", alert.Message)
	}
	return []byte(b.String())
}
