package notifier

import (
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
)

// recordingRunner records every cmdRunner invocation for assertion.
type recordingRunner struct {
	mu    sync.Mutex
	calls []runnerCall
	err   func(callNum int) error // optional per-call error injection
}

type runnerCall struct {
	args  []string
	stdin string
}

func (r *recordingRunner) run(args []string, stdin string) error {
	r.mu.Lock()
	r.calls = append(r.calls, runnerCall{args: append([]string(nil), args...), stdin: stdin})
	n := len(r.calls) - 1
	r.mu.Unlock()
	if r.err != nil {
		return r.err(n)
	}
	return nil
}

func (r *recordingRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *recordingRunner) at(i int) runnerCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[i]
}

func sampleBuildkiteAlert() evaluator.Alert {
	return evaluator.Alert{
		Rule:    "loss_spike",
		Metric:  "val_loss",
		Value:   12.4,
		Message: "val_loss spike: 12.4 on epoch 7",
		Labels: map[string]string{
			"branch": "main",
			"commit": "abc123",
			"runner": "buildkite",
		},
		FiredAt: time.Date(2026, 5, 7, 16, 18, 0, 0, time.UTC),
	}
}

// newTestBuildkiteAnnotateNotifier constructs a notifier wired to a recordingRunner with the
// `available` flag forced true (bypassing lookPath probing).
func newTestBuildkiteAnnotateNotifier(t *testing.T, style string, rec *recordingRunner) *BuildkiteAnnotateNotifier {
	t.Helper()
	if style == "" {
		style = "error"
	}
	return &BuildkiteAnnotateNotifier{
		style:     style,
		runner:    rec.run,
		available: true,
	}
}

func TestBuildkiteAnnotate_FirstSend_IncludesHeader(t *testing.T) {
	rec := &recordingRunner{}
	n := newTestBuildkiteAnnotateNotifier(t, "", rec)

	if err := n.Send(sampleBuildkiteAlert()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if rec.count() != 1 {
		t.Fatalf("expected 1 invocation, got %d", rec.count())
	}
	c := rec.at(0)

	wantArgs := []string{"buildkite-agent", "annotate", "--style", "error", "--context", "ding", "--append"}
	if !equalStringSlice(c.args, wantArgs) {
		t.Errorf("args = %v, want %v", c.args, wantArgs)
	}
	if !strings.HasPrefix(c.stdin, "# DING Alerts\n\n## loss_spike") {
		t.Errorf("stdin should start with `# DING Alerts\\n\\n## loss_spike`.\n--- got ---\n%s", c.stdin)
	}
	for _, want := range []string{
		"val_loss spike: 12.4 on epoch 7",
		"- **Metric:** `val_loss`",
		"- **Value:** `12.4`",
		"- **Fired:** `2026-05-07T16:18:00Z`",
		"- **Labels:**",
		"  - `branch`: `main`",
	} {
		if !strings.Contains(c.stdin, want) {
			t.Errorf("stdin missing expected substring %q", want)
		}
	}
}

func TestBuildkiteAnnotate_SubsequentSends_NoSecondHeader(t *testing.T) {
	rec := &recordingRunner{}
	n := newTestBuildkiteAnnotateNotifier(t, "", rec)

	a1 := sampleBuildkiteAlert()
	a2 := sampleBuildkiteAlert()
	a2.Rule = "second_alert"

	if err := n.Send(a1); err != nil {
		t.Fatalf("Send #1: %v", err)
	}
	if err := n.Send(a2); err != nil {
		t.Fatalf("Send #2: %v", err)
	}
	if rec.count() != 2 {
		t.Fatalf("expected 2 invocations, got %d", rec.count())
	}

	if !strings.Contains(rec.at(0).stdin, "# DING Alerts") {
		t.Error("first invocation should include `# DING Alerts` header")
	}
	if strings.Contains(rec.at(1).stdin, "# DING Alerts") {
		t.Errorf("second invocation should NOT include `# DING Alerts` header.\n--- got ---\n%s", rec.at(1).stdin)
	}
	if !strings.HasPrefix(rec.at(1).stdin, "## second_alert") {
		t.Errorf("second invocation should start with `## second_alert`.\n--- got ---\n%s", rec.at(1).stdin)
	}
}

func TestBuildkiteAnnotate_StyleConfigOverride(t *testing.T) {
	rec := &recordingRunner{}
	n := newTestBuildkiteAnnotateNotifier(t, "warning", rec)

	if err := n.Send(sampleBuildkiteAlert()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	args := rec.at(0).args
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--style" && args[i+1] != "warning" {
			t.Errorf("--style = %q, want warning", args[i+1])
		}
	}
}

func TestBuildkiteAnnotate_StyleDefaultsToError(t *testing.T) {
	rec := &recordingRunner{}
	n := newTestBuildkiteAnnotateNotifier(t, "", rec)

	if err := n.Send(sampleBuildkiteAlert()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	args := rec.at(0).args
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--style" && args[i+1] != "error" {
			t.Errorf("--style = %q, want error", args[i+1])
		}
	}
}

func TestBuildkiteAnnotate_ExecError_Propagates(t *testing.T) {
	rec := &recordingRunner{
		err: func(call int) error { return errors.New("agent disconnected") },
	}
	n := newTestBuildkiteAnnotateNotifier(t, "", rec)

	err := n.Send(sampleBuildkiteAlert())
	if err == nil {
		t.Fatal("expected Send to return error from runner")
	}
	if !strings.Contains(err.Error(), "buildkite_annotate Send") {
		t.Errorf("error %q should be prefixed with `buildkite_annotate Send`", err.Error())
	}
	if !strings.Contains(err.Error(), "agent disconnected") {
		t.Errorf("error %q should wrap original message", err.Error())
	}
}

func TestBuildkiteAnnotate_LabelsRenderStably(t *testing.T) {
	rec := &recordingRunner{}
	n := newTestBuildkiteAnnotateNotifier(t, "", rec)

	alert := sampleBuildkiteAlert()
	alert.Labels = map[string]string{"c": "3", "a": "1", "b": "2"}

	if err := n.Send(alert); err != nil {
		t.Fatalf("Send: %v", err)
	}
	body := rec.at(0).stdin

	idxA := strings.Index(body, "  - `a`: `1`")
	idxB := strings.Index(body, "  - `b`: `2`")
	idxC := strings.Index(body, "  - `c`: `3`")
	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("missing one or more label lines.\n--- got ---\n%s", body)
	}
	if !(idxA < idxB && idxB < idxC) {
		t.Errorf("label lines not alphabetical: a@%d b@%d c@%d", idxA, idxB, idxC)
	}
}

func TestBuildkiteAnnotate_ConcurrentSendsSerialize(t *testing.T) {
	rec := &recordingRunner{}
	n := newTestBuildkiteAnnotateNotifier(t, "", rec)

	const N = 10
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			alert := sampleBuildkiteAlert()
			alert.Rule = "rule_" + string(rune('0'+i))
			_ = n.Send(alert)
		}(i)
	}
	wg.Wait()

	if got := rec.count(); got != N {
		t.Fatalf("expected %d invocations, got %d", N, got)
	}
	for i := 0; i < N; i++ {
		body := rec.at(i).stdin
		if got := strings.Count(body, "## rule_"); got != 1 {
			t.Errorf("invocation %d body should contain exactly 1 `## rule_` heading, got %d.\n--- body ---\n%s", i, got, body)
		}
	}
}

func TestBuildkiteAnnotate_NoAgentOnPath_NoOpSend(t *testing.T) {
	rec := &recordingRunner{}
	originalLookPath := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { lookPath = originalLookPath })

	n := NewBuildkiteAnnotateNotifier("")
	n.runner = rec.run // any writes would record; we expect zero

	if err := n.Send(sampleBuildkiteAlert()); err != nil {
		t.Errorf("Send should be no-op (return nil) when buildkite-agent not on PATH; got %v", err)
	}
	if rec.count() != 0 {
		t.Errorf("expected 0 runner invocations when agent is missing, got %d", rec.count())
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
