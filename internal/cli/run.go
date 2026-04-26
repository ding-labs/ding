package cli

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"

	"github.com/zuchka/ding/internal/config"
	"github.com/zuchka/ding/internal/evaluator"
	"github.com/zuchka/ding/internal/ingester"
	"github.com/zuchka/ding/internal/metrics"
	"github.com/zuchka/ding/internal/notifier"
	"github.com/zuchka/ding/internal/runctx"
	"github.com/zuchka/ding/internal/server"
)

func newRunCmd() *cobra.Command {
	var configPath string
	var runIDOverride string

	cmd := &cobra.Command{
		Use:   "run [flags] -- <command> [args...]",
		Short: "Run a command and alert on its event stream",
		Long: `Wraps a command, captures its stdout and stderr, and evaluates DING rules
against the events the command emits. During-run alerts fire as events flow.
End-of-run rules (mode: end-of-run) fire once the wrapped command exits.

DING exits with the wrapped command's exit code. SIGTERM/SIGINT are forwarded
to the child for graceful shutdown.

Run context (run_id, runner, branch, commit, workflow, ...) is auto-detected
from common CI environment variables (GitHub Actions, GitLab CI, CircleCI,
Jenkins, Buildkite) and attached as labels to every event the rule engine
processes. A synthetic 'run.exit' event is emitted at end of run with the
exit code and duration in its Floats payload — match 'metric: run.exit' in
your rules to alert on job-level outcomes.

Lines of child output that don't parse as JSON (or Prometheus) events are
silently mirrored without ingestion, so wrapping non-instrumented commands
is safe.`,
		Example: `  # Wrap a CI step with rules from alerts.yaml
  ding run --config alerts.yaml -- pytest tests/

  # Wrap an instrumented training job
  ding run -- python train.py --epochs 100

  # Override the auto-detected run ID
  ding run --run-id manual-debug -- ./flaky-script.sh`,
		DisableFlagsInUseLine: true,
		Args:                  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(configPath, runIDOverride, args)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "ding.yaml", "path to config file")
	cmd.Flags().StringVar(&runIDOverride, "run-id", "", "override auto-detected run ID")
	return cmd
}

func runRun(configPath, runIDOverride string, args []string) error {
	collector := metrics.NewCollector()

	eng, cfg, notifiers, alertLogger, jqCode, err := server.BuildFromConfig(configPath, collector)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	defer func() {
		// Drain notifiers before stopping so queued deliveries complete before
		// the process exits. Falls back to Stop() for notifiers without Drain.
		for _, n := range notifiers {
			if drainer, ok := n.(interface{ Drain(time.Duration) }); ok {
				drainer.Drain(cfg.Server.DrainTimeout.Duration)
			} else if stopper, ok := n.(interface{ Stop() }); ok {
				stopper.Stop()
			}
		}
		if alertLogger != nil {
			_ = alertLogger.Close()
		}
	}()

	rc := runctx.New()
	if runIDOverride != "" {
		rc.RunID = runIDOverride
	}

	log.Printf("ding: run start — run_id=%s runner=%s cmd=%v", rc.RunID, rc.Runner, args)

	cmdRun := exec.Command(args[0], args[1:]...)
	cmdRun.Env = os.Environ()
	stdoutPipe, err := cmdRun.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmdRun.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmdRun.Start(); err != nil {
		return fmt.Errorf("starting command: %w", err)
	}

	// Forward SIGTERM/SIGINT to the child so it can shut down gracefully.
	// The child's exit will then unblock cmdRun.Wait().
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	signalsDone := make(chan struct{})
	go func() {
		defer close(signalsDone)
		for sig := range sigCh {
			if cmdRun.Process != nil {
				_ = cmdRun.Process.Signal(sig)
			}
		}
	}()

	// Mirror+ingest both streams. Both must finish before cmdRun.Wait()
	// returns successfully because Wait closes the pipes.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ingestStream(stdoutPipe, os.Stdout, eng, notifiers, alertLogger, cfg, jqCode, rc)
	}()
	go func() {
		defer wg.Done()
		ingestStream(stderrPipe, os.Stderr, eng, notifiers, alertLogger, cfg, jqCode, rc)
	}()

	wg.Wait()
	waitErr := cmdRun.Wait()

	// Stop signal forwarding before we exit.
	signal.Stop(sigCh)
	close(sigCh)
	<-signalsDone

	exitCode := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			log.Printf("ding: command error: %v", waitErr)
			exitCode = 1
		}
	}

	// Synthetic run.exit event flows through the engine like any other —
	// during-run rules matching metric: run.exit fire here.
	summary := rc.SummaryEvent(exitCode)
	dispatchEvent(summary, eng, notifiers, alertLogger)

	// End-of-run rules accumulate state during the run; fire them now.
	endAlerts := eng.ProcessEndOfRun(time.Now())
	dispatchAlerts(endAlerts, notifiers, alertLogger)

	log.Printf("ding: run end — run_id=%s exit_code=%d duration=%.1fs",
		rc.RunID, exitCode, time.Since(rc.StartedAt).Seconds())

	// Mirror the child's exit code so callers (CI runners, parent processes)
	// see the same status they would have seen running the command directly.
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

// ingestStream reads from r line-by-line, mirrors each line to mirror, and
// attempts to parse it as a DING event (JSON or Prometheus). Successfully
// parsed events get run-context labels applied and flow through the engine.
// Unparseable lines are silently mirrored only — this is the canonical
// "wrap any command, instrument later" affordance.
func ingestStream(
	r io.Reader,
	mirror io.Writer,
	eng *evaluator.Engine,
	notifiers map[string]notifier.Notifier,
	alertLogger *notifier.AlertLogger,
	cfg *config.Config,
	jqCode *gojq.Code,
	rc *runctx.Context,
) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()

		// Always mirror so users see their command's output. Even non-event
		// lines (test logs, shell prompts) pass through cleanly.
		_, _ = mirror.Write(line)
		_, _ = mirror.Write([]byte{'\n'})

		if len(line) == 0 {
			continue
		}

		var events []ingester.Event
		var err error
		if jqCode != nil {
			events, err = ingester.RunJQ(jqCode, line)
		} else {
			format := ingester.DetectFormat(line, "", cfg.Server.Format)
			if format == "json" {
				events, err = ingester.ParseJSONLine(line)
			} else {
				events, err = ingester.ParsePrometheusText(line)
			}
		}
		if err != nil || len(events) == 0 {
			continue
		}
		for _, ev := range events {
			ev.Labels = rc.Apply(ev.Labels)
			dispatchEvent(ev, eng, notifiers, alertLogger)
		}
	}
	// Scanner errors on closed pipes are expected at EOF; only log surprising ones.
	if err := scanner.Err(); err != nil {
		log.Printf("ding: ingest scan error: %v", err)
	}
}

func dispatchEvent(
	ev ingester.Event,
	eng *evaluator.Engine,
	notifiers map[string]notifier.Notifier,
	alertLogger *notifier.AlertLogger,
) {
	alerts := eng.Process(ev, time.Now())
	dispatchAlerts(alerts, notifiers, alertLogger)
}

func dispatchAlerts(
	alerts []evaluator.Alert,
	notifiers map[string]notifier.Notifier,
	alertLogger *notifier.AlertLogger,
) {
	for _, alert := range alerts {
		if alertLogger != nil {
			if err := alertLogger.Log(alert); err != nil {
				log.Printf("ding: alert log write error: %v", err)
			}
		}
		for _, name := range alert.Notifiers {
			n, ok := notifiers[name]
			if !ok {
				log.Printf("ding: unknown notifier %q for rule %q", name, alert.Rule)
				continue
			}
			if err := n.Send(alert); err != nil {
				log.Printf("ding: notifier %q error: %v", name, err)
			}
		}
	}
}
