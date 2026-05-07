package notifier

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
)

// cmdRunner runs the buildkite-agent CLI. Tests inject a stub.
type cmdRunner func(args []string, stdin string) error

func defaultCmdRunner(args []string, stdin string) error {
	if len(args) == 0 {
		return fmt.Errorf("cmdRunner: empty args")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w (stderr: %s)", err, msg)
	}
	return nil
}

// lookPath is overridable for tests.
var lookPath = exec.LookPath

// BuildkiteAnnotateNotifier publishes DING alerts as Buildkite build
// annotations by shelling out to `buildkite-agent annotate`. All alerts
// for a build land in a single rolling annotation (--context ding
// --append). First Send writes a `# DING Alerts` H1 header; subsequent
// Sends append `## <rule>` sections only — Buildkite's --append mode
// concatenates each invocation's stdin into the existing annotation body.
//
// Sync, mutex-guarded. Outside Buildkite (buildkite-agent not on PATH),
// logs once at construction and Send becomes a no-op — same graceful-
// degrade philosophy as the github_actions notifier.
type BuildkiteAnnotateNotifier struct {
	mu          sync.Mutex
	style       string
	runner      cmdRunner
	available   bool // false if buildkite-agent not on PATH at construction
	wroteHeader bool
}

// NewBuildkiteAnnotateNotifier constructs a notifier that publishes DING
// alerts as Buildkite annotations. style defaults to "error" if empty;
// validated by config.Validate. Caller is responsible for ensuring style
// is one of "success", "info", "warning", "error".
func NewBuildkiteAnnotateNotifier(style string) *BuildkiteAnnotateNotifier {
	if style == "" {
		style = "error"
	}
	n := &BuildkiteAnnotateNotifier{
		style:  style,
		runner: defaultCmdRunner,
	}
	if _, err := lookPath("buildkite-agent"); err != nil {
		log.Printf("ding: buildkite_annotate notifier: buildkite-agent not on PATH; alerts via this notifier will be no-ops")
		n.available = false
	} else {
		n.available = true
	}
	return n
}

// Send invokes `buildkite-agent annotate --style <style> --context ding
// --append` with the rendered Markdown as stdin. No-op (returns nil)
// if buildkite-agent wasn't on PATH at construction.
func (n *BuildkiteAnnotateNotifier) Send(alert evaluator.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.available {
		return nil
	}

	body := n.renderBody(alert)
	args := []string{"buildkite-agent", "annotate", "--style", n.style, "--context", "ding", "--append"}
	if err := n.runner(args, body); err != nil {
		return fmt.Errorf("buildkite_annotate Send: %w", err)
	}
	return nil
}

// renderBody produces the per-invocation stdin payload. Caller must hold n.mu;
// modifies n.wroteHeader.
func (n *BuildkiteAnnotateNotifier) renderBody(alert evaluator.Alert) string {
	var b strings.Builder
	if !n.wroteHeader {
		b.WriteString("# DING Alerts\n\n")
		n.wroteHeader = true
	}
	fmt.Fprintf(&b, "## %s\n\n", alert.Rule)
	if alert.Message != "" {
		fmt.Fprintf(&b, "%s\n\n", alert.Message)
	}
	fmt.Fprintf(&b, "- **Metric:** `%s`\n", alert.Metric)
	fmt.Fprintf(&b, "- **Value:** `%v`\n", alert.Value)
	fmt.Fprintf(&b, "- **Fired:** `%s`\n", alert.FiredAt.Format(time.RFC3339))
	if alert.Count > 0 || alert.Avg != 0 || alert.Sum != 0 {
		fmt.Fprintf(&b, "- **Aggregates:** count=`%v` avg=`%v` min=`%v` max=`%v` sum=`%v`\n",
			alert.Count, alert.Avg, alert.Min, alert.Max, alert.Sum)
	}
	if len(alert.Labels) > 0 {
		b.WriteString("- **Labels:**\n")
		// Stable order so concatenated annotation diffs cleanly between Sends.
		keys := make([]string, 0, len(alert.Labels))
		for k := range alert.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  - `%s`: `%s`\n", k, alert.Labels[k])
		}
	}
	b.WriteString("\n")
	return b.String()
}
