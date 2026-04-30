package notifier

import (
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
)

// GitHubActionsNotifier emits alerts as GitHub Actions step-summary markdown
// (when running inside Actions, $GITHUB_STEP_SUMMARY points to a writable
// file) and, regardless of summary availability, also emits inline
// `::warning::` workflow commands on stdout so alerts surface in the live
// log and the run annotations panel.
//
// Falls back gracefully when run outside Actions (no env var set): just
// the workflow command lines. The GHA log treats them as plain text in
// non-Actions contexts so this is harmless.
type GitHubActionsNotifier struct {
	mu          sync.Mutex
	summaryPath string
	stdout      io.Writer
	wroteHeader bool
}

// NewGitHubActionsNotifier returns a notifier that auto-detects the GHA
// step summary path from the environment. If stdout is nil, os.Stdout is used
// for inline workflow commands.
func NewGitHubActionsNotifier(stdout io.Writer) *GitHubActionsNotifier {
	if stdout == nil {
		stdout = os.Stdout
	}
	return &GitHubActionsNotifier{
		summaryPath: os.Getenv("GITHUB_STEP_SUMMARY"),
		stdout:      stdout,
	}
}

// Send writes the alert to both the step summary (if available) and stdout
// as a workflow command. Errors writing the summary are returned; stdout
// errors are ignored because they typically mean a closed pipe at shutdown.
func (n *GitHubActionsNotifier) Send(alert evaluator.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Inline annotation. GHA recognizes `::warning::` and renders it as a
	// run-level annotation.
	fmt.Fprintf(n.stdout, "::warning title=DING %s::%s\n", escapeAnnotation(alert.Rule), escapeAnnotation(alert.Message))

	if n.summaryPath == "" {
		return nil
	}

	f, err := os.OpenFile(n.summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening github step summary: %w", err)
	}
	defer f.Close()

	if !n.wroteHeader {
		fmt.Fprintln(f, "## DING Alerts")
		fmt.Fprintln(f)
		n.wroteHeader = true
	}
	fmt.Fprintf(f, "### %s\n\n", alert.Rule)
	if alert.Message != "" {
		fmt.Fprintf(f, "%s\n\n", alert.Message)
	}
	fmt.Fprintf(f, "- **Metric:** `%s`\n", alert.Metric)
	fmt.Fprintf(f, "- **Value:** `%v`\n", alert.Value)
	fmt.Fprintf(f, "- **Fired:** `%s`\n", alert.FiredAt.Format(time.RFC3339))
	if alert.Count > 0 || alert.Avg != 0 || alert.Sum != 0 {
		fmt.Fprintf(f, "- **Aggregates:** count=`%v` avg=`%v` min=`%v` max=`%v` sum=`%v`\n",
			alert.Count, alert.Avg, alert.Min, alert.Max, alert.Sum)
	}
	if len(alert.Labels) > 0 {
		fmt.Fprintln(f, "- **Labels:**")
		// Stable order so summaries diff cleanly.
		keys := make([]string, 0, len(alert.Labels))
		for k := range alert.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(f, "  - `%s`: `%s`\n", k, alert.Labels[k])
		}
	}
	fmt.Fprintln(f)
	return nil
}

// escapeAnnotation replaces characters that GHA treats specially in workflow
// command arguments so the visible message stays readable.
// See https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions
func escapeAnnotation(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '%':
			out = append(out, '%', '2', '5')
		case '\r':
			out = append(out, '%', '0', 'D')
		case '\n':
			out = append(out, '%', '0', 'A')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
