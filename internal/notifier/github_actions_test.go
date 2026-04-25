package notifier

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zuchka/ding/internal/evaluator"
)

func TestGitHubActionsNotifier_WritesSummaryAndStdout(t *testing.T) {
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	var stdout bytes.Buffer
	n := NewGitHubActionsNotifier(&stdout)

	alert := evaluator.Alert{
		Rule:    "high_latency",
		Message: "p99 was 1234ms",
		Metric:  "latency",
		Value:   1234,
		Labels:  map[string]string{"run_id": "r-1", "branch": "main"},
		FiredAt: time.Now(),
	}

	if err := n.Send(alert); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Stdout should have the workflow command annotation.
	if !strings.Contains(stdout.String(), "::warning title=DING high_latency::p99 was 1234ms") {
		t.Errorf("stdout missing workflow command; got: %q", stdout.String())
	}

	// Summary file should have the markdown rendering.
	body, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("reading summary: %v", err)
	}
	bodyStr := string(body)
	for _, want := range []string{
		"## DING Alerts",
		"### high_latency",
		"p99 was 1234ms",
		"`latency`",
		"`run_id`: `r-1`",
		"`branch`: `main`",
	} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("summary missing %q; got:\n%s", want, bodyStr)
		}
	}
}

func TestGitHubActionsNotifier_NoSummaryEnvVar(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", "")

	var stdout bytes.Buffer
	n := NewGitHubActionsNotifier(&stdout)

	alert := evaluator.Alert{
		Rule:    "test",
		Message: "msg",
		FiredAt: time.Now(),
	}

	if err := n.Send(alert); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !strings.Contains(stdout.String(), "::warning") {
		t.Errorf("stdout missing workflow command; got: %q", stdout.String())
	}
}

func TestGitHubActionsNotifier_HeaderOnce(t *testing.T) {
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	n := NewGitHubActionsNotifier(&bytes.Buffer{})

	for i := 0; i < 3; i++ {
		if err := n.Send(evaluator.Alert{Rule: "r", Message: "m", FiredAt: time.Now()}); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}

	body, _ := os.ReadFile(summaryPath)
	if got := strings.Count(string(body), "## DING Alerts"); got != 1 {
		t.Errorf("expected exactly 1 '## DING Alerts' header, got %d", got)
	}
}

func TestEscapeAnnotation(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"50%", "50%25"},
		{"a\nb", "a%0Ab"},
		{"a\rb", "a%0Db"},
	}
	for _, c := range cases {
		if got := escapeAnnotation(c.in); got != c.want {
			t.Errorf("escapeAnnotation(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
