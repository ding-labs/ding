package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ding-labs/ding/internal/dryrun"
	"github.com/ding-labs/ding/internal/ingester"
	"github.com/ding-labs/ding/internal/runctx"
	"github.com/ding-labs/ding/internal/server"
)

func newTestRuleCmd() *cobra.Command {
	var configPath string
	var format string
	var noColor bool

	cmd := &cobra.Command{
		Use:   "test-rule [FILE]",
		Short: "Replay JSONL events through the rule engine without sending notifications",
		Long: `Reads JSONL events (one per line) from FILE or stdin and replays them
through the rule engine using your config. Matching rules render their
messages as if they were about to fire, but no notifier sends happen.

Each event is a normal DING JSON event — same shape DING already parses
on 'ding run' stdin. An optional 'timestamp' field (RFC3339 or Unix epoch)
controls the event's time for windowed rules; omitted timestamps get
synthesized sequentially starting from now.

End-of-run rules (mode: end-of-run) fire after all input events are
consumed.`,
		Example: `  # Replay events from a file
  ding test-rule events.jsonl

  # Pipe events from another tool
  cat events.jsonl | ding test-rule

  # Force JSON output (default is text on TTY, json when piped)
  ding test-rule events.jsonl --format json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input, closer, err := openTestRuleInput(args)
			if err != nil {
				return err
			}
			if closer != nil {
				defer closer()
			}
			return runTestRule(configPath, format, noColor, input, os.Stdout, os.Stderr)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "ding.yaml", "path to config file")
	cmd.Flags().StringVar(&format, "format", "auto", "output format: auto, text, json")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI color in text format")
	return cmd
}

func openTestRuleInput(args []string) (io.Reader, func(), error) {
	if len(args) == 0 || args[0] == "-" {
		return os.Stdin, nil, nil
	}
	f, err := os.Open(args[0])
	if err != nil {
		return nil, nil, fmt.Errorf("opening events file: %w", err)
	}
	return f, func() { _ = f.Close() }, nil
}

func runTestRule(configPath, format string, noColor bool, input io.Reader, stdout, stderr io.Writer) error {
	eng, _, _, _, _, err := server.BuildFromConfig(configPath, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	rc := runctx.New()
	if rc.Runner == "local" {
		fmt.Fprintln(stderr, "ding: note — runner=local; rules matching on a specific runner label will not match.")
	}

	formatter := pickFormatter(format, noColor, stdout)
	dispatcher := dryrun.NewLoggingDispatcher(formatter, stdout)

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	syntheticBase := time.Now()
	idx := 0
	var lastAt time.Time
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		events, err := ingester.ParseJSONLine(line)
		if err != nil || len(events) == 0 {
			fmt.Fprintf(stderr, "ding: skipping unparseable event line %d: %v\n", idx+1, err)
			idx++
			continue
		}
		for _, ev := range events {
			if ev.At.IsZero() {
				ev.At = syntheticBase.Add(time.Duration(idx) * time.Second)
			} else {
				// ParseJSONLine only handles Unix epoch timestamps; try RFC3339 override.
				ev.At = resolveTimestamp(line, ev.At)
			}
			ev.Labels = rc.Apply(ev.Labels)
			lastAt = ev.At
			alerts := eng.Process(ev, ev.At)
			dispatcher.Dispatch(alerts)
		}
		idx++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	endTime := lastAt
	if endTime.IsZero() {
		endTime = time.Now()
	}
	endAlerts := eng.ProcessEndOfRun(endTime)
	dispatcher.Dispatch(endAlerts)
	return nil
}

// resolveTimestamp tries to parse a "timestamp" field from raw JSON as RFC3339.
// If successful it returns the parsed time; otherwise it returns the fallback.
// This extends ParseJSONLine (which handles Unix epoch floats) to also accept
// RFC3339 strings — useful for test fixtures and replayed log lines.
func resolveTimestamp(raw []byte, fallback time.Time) time.Time {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fallback
	}
	tsRaw, ok := obj["timestamp"]
	if !ok {
		return fallback
	}
	var s string
	if err := json.Unmarshal(tsRaw, &s); err != nil {
		return fallback
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fallback
	}
	return t
}

func pickFormatter(format string, noColor bool, out io.Writer) dryrun.Formatter {
	if format == "json" {
		return &dryrun.JSONFormatter{}
	}
	if format == "text" {
		return &dryrun.TextFormatter{Color: !noColor && isTerminal(out)}
	}
	// auto: JSON when piped, text when on TTY
	if isTerminal(out) {
		return &dryrun.TextFormatter{Color: !noColor}
	}
	return &dryrun.JSONFormatter{}
}

// isTerminal reports whether w is an *os.File pointing at a terminal.
// Returns false for buffers, pipes, and any non-*os.File writer.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
