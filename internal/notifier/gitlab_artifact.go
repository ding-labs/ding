package notifier

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
)

// GitLabArtifactNotifier writes alert Markdown to a file the user declares
// in their .gitlab-ci.yml `artifacts:` block, so DING alerts surface as a
// downloadable / browsable artifact in the GitLab job UI without an external
// service. Closes the "no native step-summary surface" tradeoff documented
// in docs/recipes/gitlab-ci.md.
//
// Sync, mutex-guarded, append-only. First call writes `# DING Alerts` once;
// subsequent calls only append per-alert sections. Falls back gracefully
// outside GitLab CI: just produces a local file, harmless.
type GitLabArtifactNotifier struct {
	mu          sync.Mutex
	path        string
	wroteHeader bool
}

// NewGitLabArtifactNotifier returns a notifier that writes alert Markdown
// to path. If path is empty, defaults to "ding-alerts.md" in the current
// working directory (= $CI_PROJECT_DIR in GitLab CI).
func NewGitLabArtifactNotifier(path string) *GitLabArtifactNotifier {
	if path == "" {
		path = "ding-alerts.md"
	}
	return &GitLabArtifactNotifier{path: path}
}

// Send appends an alert section to the artifact file. On first call also
// writes the `# DING Alerts` H1 header. Returns an error if the file can't
// be opened or written (e.g. non-writable parent directory).
func (n *GitLabArtifactNotifier) Send(alert evaluator.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	f, err := os.OpenFile(n.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening gitlab artifact file %q: %w", n.path, err)
	}
	defer f.Close()

	if !n.wroteHeader {
		fmt.Fprintln(f, "# DING Alerts")
		fmt.Fprintln(f)
		n.wroteHeader = true
	}

	fmt.Fprintf(f, "## %s\n\n", alert.Rule)
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
		// Stable order so artifact diffs cleanly between runs.
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
