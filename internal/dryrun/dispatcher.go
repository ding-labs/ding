package dryrun

import (
	"io"

	"github.com/ding-labs/ding/internal/evaluator"
)

// LoggingDispatcher formats each alert via Formatter and writes to Writer.
// Never calls notifier.Send. Satisfies cli.Dispatcher implicitly (Go duck-typing).
type LoggingDispatcher struct {
	Formatter Formatter
	Writer    io.Writer
}

func NewLoggingDispatcher(f Formatter, w io.Writer) *LoggingDispatcher {
	return &LoggingDispatcher{Formatter: f, Writer: w}
}

func (d *LoggingDispatcher) Dispatch(alerts []evaluator.Alert) {
	for _, alert := range alerts {
		_, _ = d.Writer.Write(d.Formatter.Format(alert))
	}
}
