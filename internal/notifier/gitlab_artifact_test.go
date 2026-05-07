package notifier

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
)

func sampleGitLabArtifactAlert() evaluator.Alert {
	return evaluator.Alert{
		Rule:    "loss_spike",
		Metric:  "val_loss",
		Value:   12.4,
		Message: "val_loss spike: 12.4 on epoch 7",
		Labels: map[string]string{
			"branch": "main",
			"commit": "abc123",
			"runner": "gitlab-ci",
		},
		FiredAt: time.Date(2026, 5, 7, 16, 18, 0, 0, time.UTC),
	}
}

func TestGitLabArtifact_WritesFirstAlert_IncludesHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ding-alerts.md")
	n := NewGitLabArtifactNotifier(path)

	if err := n.Send(sampleGitLabArtifactAlert()); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(contents)

	if !strings.HasPrefix(body, "# DING Alerts\n\n") {
		t.Errorf("file does not start with `# DING Alerts` header.\n--- got ---\n%s", body)
	}
	for _, want := range []string{
		"## loss_spike",
		"val_loss spike: 12.4 on epoch 7",
		"- **Metric:** `val_loss`",
		"- **Value:** `12.4`",
		"- **Fired:** `2026-05-07T16:18:00Z`",
		"- **Labels:**",
		"  - `branch`: `main`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("file missing expected substring %q.\n--- got ---\n%s", want, body)
		}
	}
}

func TestGitLabArtifact_AppendsSubsequentAlerts_NoSecondHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ding-alerts.md")
	n := NewGitLabArtifactNotifier(path)

	a1 := sampleGitLabArtifactAlert()
	a2 := sampleGitLabArtifactAlert()
	a2.Rule = "second_alert"
	a2.Message = "another alert message"

	if err := n.Send(a1); err != nil {
		t.Fatalf("Send #1: %v", err)
	}
	if err := n.Send(a2); err != nil {
		t.Fatalf("Send #2: %v", err)
	}

	contents, _ := os.ReadFile(path)
	body := string(contents)
	if got := strings.Count(body, "# DING Alerts"); got != 1 {
		t.Errorf("expected exactly 1 `# DING Alerts` header, got %d.\n--- got ---\n%s", got, body)
	}
	if got := strings.Count(body, "## loss_spike"); got != 1 {
		t.Errorf("expected exactly 1 `## loss_spike` section, got %d", got)
	}
	if got := strings.Count(body, "## second_alert"); got != 1 {
		t.Errorf("expected exactly 1 `## second_alert` section, got %d", got)
	}
}

func TestGitLabArtifact_DefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	n := NewGitLabArtifactNotifier("") // empty path → defaults to "ding-alerts.md"
	if err := n.Send(sampleGitLabArtifactAlert()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(dir, "ding-alerts.md"))
	if err != nil {
		t.Fatalf("ReadFile default path: %v", err)
	}
	if !strings.Contains(string(contents), "# DING Alerts") {
		t.Errorf("default-path file missing header.\n--- got ---\n%s", string(contents))
	}
}

func TestGitLabArtifact_FileWriteError(t *testing.T) {
	dir := t.TempDir()
	// Make the directory non-writable so OpenFile fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	path := filepath.Join(dir, "ding-alerts.md")
	n := NewGitLabArtifactNotifier(path)

	err := n.Send(sampleGitLabArtifactAlert())
	if err == nil {
		t.Fatal("expected Send to return an error for non-writable path, got nil")
	}
	if !strings.Contains(err.Error(), "gitlab artifact") {
		t.Errorf("error message %q should mention 'gitlab artifact' for context", err.Error())
	}
}

func TestGitLabArtifact_LabelsRenderStably(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ding-alerts.md")
	n := NewGitLabArtifactNotifier(path)

	alert := sampleGitLabArtifactAlert()
	alert.Labels = map[string]string{"c": "3", "a": "1", "b": "2"}

	if err := n.Send(alert); err != nil {
		t.Fatalf("Send: %v", err)
	}

	contents, _ := os.ReadFile(path)
	body := string(contents)

	idxA := strings.Index(body, "  - `a`: `1`")
	idxB := strings.Index(body, "  - `b`: `2`")
	idxC := strings.Index(body, "  - `c`: `3`")
	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("missing one or more label lines.\n--- got ---\n%s", body)
	}
	if !(idxA < idxB && idxB < idxC) {
		t.Errorf("label lines not in alphabetical order: a@%d b@%d c@%d", idxA, idxB, idxC)
	}
}

func TestGitLabArtifact_HandlesEmptyMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ding-alerts.md")
	n := NewGitLabArtifactNotifier(path)

	alert := sampleGitLabArtifactAlert()
	alert.Message = ""

	if err := n.Send(alert); err != nil {
		t.Fatalf("Send: %v", err)
	}

	contents, _ := os.ReadFile(path)
	body := string(contents)

	// After "## loss_spike\n\n" there should be no message paragraph;
	// the next non-empty content is "- **Metric:**".
	header := "## loss_spike\n\n"
	idx := strings.Index(body, header)
	if idx < 0 {
		t.Fatalf("missing rule header.\n--- got ---\n%s", body)
	}
	rest := body[idx+len(header):]
	if !strings.HasPrefix(rest, "- **Metric:**") {
		t.Errorf("expected `- **Metric:**` immediately after rule header (no empty message paragraph).\n--- got rest ---\n%s", rest)
	}
}

func TestGitLabArtifact_ConcurrentSendsSerialize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ding-alerts.md")
	n := NewGitLabArtifactNotifier(path)

	const N = 10
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			alert := sampleGitLabArtifactAlert()
			alert.Rule = "rule_" + string(rune('0'+i))
			alert.Message = "msg for " + alert.Rule
			_ = n.Send(alert)
		}(i)
	}
	wg.Wait()

	contents, _ := os.ReadFile(path)
	body := string(contents)

	if got := strings.Count(body, "# DING Alerts"); got != 1 {
		t.Errorf("expected exactly 1 H1 header, got %d", got)
	}
	for i := 0; i < N; i++ {
		want := "## rule_" + string(rune('0'+i))
		if !strings.Contains(body, want) {
			t.Errorf("missing section %q from concurrent Send", want)
		}
	}
}
