package runctx

import (
	"testing"
	"time"
)

func TestNew_LocalDefault(t *testing.T) {
	clearCIEnv(t)

	c := New()
	if c.Runner != "local" {
		t.Errorf("Runner = %q, want local", c.Runner)
	}
	if c.RunID == "" {
		t.Error("RunID should be auto-generated when no CI env present")
	}
	if c.Labels == nil {
		t.Error("Labels should be non-nil even with no CI env")
	}
}

func TestNew_DetectsGitHubActions(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "42")
	t.Setenv("GITHUB_REPOSITORY", "zuchka/ding")
	t.Setenv("GITHUB_REF_NAME", "main")
	t.Setenv("GITHUB_SHA", "abc123")
	t.Setenv("GITHUB_WORKFLOW", "ci")

	c := New()

	if c.Runner != "github-actions" {
		t.Errorf("Runner = %q, want github-actions", c.Runner)
	}
	if c.RunID != "42" {
		t.Errorf("RunID = %q, want 42", c.RunID)
	}
	wantLabels := map[string]string{
		"repo":     "zuchka/ding",
		"branch":   "main",
		"commit":   "abc123",
		"workflow": "ci",
	}
	for k, v := range wantLabels {
		if got := c.Labels[k]; got != v {
			t.Errorf("Labels[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestNew_DetectsGitLabCI(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_PIPELINE_ID", "999")
	t.Setenv("CI_PROJECT_PATH", "group/proj")
	t.Setenv("CI_COMMIT_REF_NAME", "feature/x")

	c := New()

	if c.Runner != "gitlab-ci" {
		t.Errorf("Runner = %q, want gitlab-ci", c.Runner)
	}
	if c.RunID != "999" {
		t.Errorf("RunID = %q, want 999", c.RunID)
	}
	if c.Labels["repo"] != "group/proj" || c.Labels["branch"] != "feature/x" {
		t.Errorf("labels not propagated: %#v", c.Labels)
	}
}

func TestNew_DetectsCircleCI(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("CIRCLECI", "true")
	t.Setenv("CIRCLE_BUILD_NUM", "7")
	t.Setenv("CIRCLE_BRANCH", "main")

	c := New()
	if c.Runner != "circleci" {
		t.Errorf("Runner = %q, want circleci", c.Runner)
	}
	if c.RunID != "7" {
		t.Errorf("RunID = %q, want 7", c.RunID)
	}
}

func TestNew_DetectsJenkins(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("JENKINS_URL", "http://jenkins.local")
	t.Setenv("BUILD_TAG", "jenkins-job-1")
	t.Setenv("JOB_NAME", "build")

	c := New()
	if c.Runner != "jenkins" {
		t.Errorf("Runner = %q, want jenkins", c.Runner)
	}
	if c.RunID != "jenkins-job-1" {
		t.Errorf("RunID = %q, want jenkins-job-1", c.RunID)
	}
}

func TestNew_DetectsKubernetes(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("POD_UID", "abc-def-123")
	t.Setenv("POD_NAME", "my-job-pod-xyz")
	t.Setenv("POD_NAMESPACE", "production")
	t.Setenv("NODE_NAME", "ip-10-0-1-100.ec2.internal")
	t.Setenv("JOB_NAME", "nightly-batch")

	c := New()

	if c.Runner != "kubernetes" {
		t.Errorf("Runner = %q, want kubernetes", c.Runner)
	}
	if c.RunID != "abc-def-123" {
		t.Errorf("RunID = %q, want abc-def-123 (POD_UID)", c.RunID)
	}
	wantLabels := map[string]string{
		"namespace": "production",
		"pod":       "my-job-pod-xyz",
		"node":      "ip-10-0-1-100.ec2.internal",
		"job_name":  "nightly-batch",
	}
	for k, v := range wantLabels {
		if got := c.Labels[k]; got != v {
			t.Errorf("Labels[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestNew_KubernetesFallsBackToPodNameWhenNoUID(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("POD_NAME", "my-job-pod-xyz")
	// POD_UID intentionally unset — exercise fallback.

	c := New()
	if c.Runner != "kubernetes" {
		t.Errorf("Runner = %q, want kubernetes", c.Runner)
	}
	if c.RunID != "my-job-pod-xyz" {
		t.Errorf("RunID = %q, want my-job-pod-xyz (POD_NAME fallback)", c.RunID)
	}
}

// TestNew_CIDetectionWinsOverKubernetes locks in the design contract that
// CI runner detection takes precedence over bare Kubernetes detection. A
// self-hosted GitHub Actions runner (or Jenkins agent, etc.) deployed on
// K8s sets BOTH the CI env vars AND KUBERNETES_SERVICE_HOST; CI context is
// richer for alerting purposes, so it must win. Regression guard.
func TestNew_CIDetectionWinsOverKubernetes(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("POD_NAME", "k8s-runner-pod")
	t.Setenv("POD_NAMESPACE", "actions-runner-system")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "555")
	t.Setenv("GITHUB_REPOSITORY", "zuchka/ding")

	c := New()

	if c.Runner != "github-actions" {
		t.Errorf("Runner = %q, want github-actions (CI must win over K8s)", c.Runner)
	}
	if c.RunID != "555" {
		t.Errorf("RunID = %q, want 555", c.RunID)
	}
	if _, ok := c.Labels["namespace"]; ok {
		t.Errorf("namespace leaked from K8s detection — CI should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["pod"]; ok {
		t.Errorf("pod leaked from K8s detection — CI should win cleanly: %#v", c.Labels)
	}
}

func TestApply_AddsRunContextLabels(t *testing.T) {
	clearCIEnv(t)
	c := &Context{
		RunID:  "r1",
		Runner: "local",
		Labels: map[string]string{"branch": "main"},
	}

	out := c.Apply(map[string]string{"host": "web-01"})

	if out["run_id"] != "r1" || out["runner"] != "local" || out["branch"] != "main" || out["host"] != "web-01" {
		t.Errorf("unexpected labels: %#v", out)
	}
}

func TestApply_DoesNotClobberExistingLabels(t *testing.T) {
	c := &Context{
		RunID:  "r1",
		Runner: "github-actions",
		Labels: map[string]string{"branch": "from-ci"},
	}

	// Caller already set branch — must not be overwritten by run context.
	out := c.Apply(map[string]string{"branch": "user-supplied"})

	if out["branch"] != "user-supplied" {
		t.Errorf("Apply clobbered existing label: branch = %q", out["branch"])
	}
}

func TestApply_NilInput(t *testing.T) {
	c := &Context{RunID: "r1", Runner: "local"}
	out := c.Apply(nil)
	if out == nil {
		t.Fatal("Apply(nil) should return a non-nil map")
	}
	if out["run_id"] != "r1" {
		t.Errorf("run_id missing: %#v", out)
	}
}

func TestSummaryEvent_PopulatesFields(t *testing.T) {
	c := &Context{
		RunID:     "r1",
		Runner:    "local",
		StartedAt: time.Now().Add(-3 * time.Second),
		Labels:    map[string]string{},
	}

	ev := c.SummaryEvent(2)

	if ev.Metric != "run.exit" {
		t.Errorf("Metric = %q, want run.exit", ev.Metric)
	}
	if ev.Value != 2 {
		t.Errorf("Value = %v, want 2", ev.Value)
	}
	if ev.Floats["exit_code"] != 2 {
		t.Errorf("Floats[exit_code] = %v, want 2", ev.Floats["exit_code"])
	}
	if ev.Floats["duration_seconds"] < 3 {
		t.Errorf("duration_seconds = %v, want >= 3", ev.Floats["duration_seconds"])
	}
	if ev.Labels["run_id"] != "r1" || ev.Labels["runner"] != "local" {
		t.Errorf("run labels missing: %#v", ev.Labels)
	}
	if ev.Labels["exit_code"] != "2" {
		t.Errorf("exit_code label = %q, want \"2\"", ev.Labels["exit_code"])
	}
	if c.ExitCode == nil || *c.ExitCode != 2 {
		t.Error("ExitCode not recorded on context")
	}
	if c.EndedAt == nil {
		t.Error("EndedAt not recorded on context")
	}
}

func TestGenerateID_Unique(t *testing.T) {
	a := generateID()
	b := generateID()
	if a == b {
		t.Error("generateID returned identical values")
	}
	if len(a) != 16 {
		t.Errorf("generateID len = %d, want 16 hex chars", len(a))
	}
}

// clearCIEnv unsets all CI/runner env vars so detection starts from a clean slate.
// Uses t.Setenv so they're restored after the test.
func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GITHUB_ACTIONS", "GITHUB_RUN_ID", "GITHUB_REPOSITORY", "GITHUB_REF_NAME",
		"GITHUB_SHA", "GITHUB_WORKFLOW", "GITHUB_JOB", "GITHUB_ACTOR", "GITHUB_EVENT_NAME",
		"GITLAB_CI", "CI_PIPELINE_ID", "CI_PROJECT_PATH", "CI_COMMIT_REF_NAME",
		"CI_COMMIT_SHA", "CI_JOB_NAME",
		"CIRCLECI", "CIRCLE_BUILD_NUM", "CIRCLE_PROJECT_REPONAME", "CIRCLE_BRANCH",
		"CIRCLE_SHA1", "CIRCLE_JOB",
		"JENKINS_URL", "BUILD_TAG", "JOB_NAME", "BUILD_NUMBER",
		"BUILDKITE", "BUILDKITE_BUILD_ID", "BUILDKITE_PIPELINE_SLUG",
		"BUILDKITE_BRANCH", "BUILDKITE_COMMIT",
		"KUBERNETES_SERVICE_HOST", "POD_UID", "POD_NAME", "POD_NAMESPACE", "NODE_NAME",
	} {
		t.Setenv(k, "")
	}
}
