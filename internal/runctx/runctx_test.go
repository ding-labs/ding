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
	t.Setenv("GITHUB_REPOSITORY", "ding-labs/ding")
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
		"repo":     "ding-labs/ding",
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
	t.Setenv("GITHUB_REPOSITORY", "ding-labs/ding")

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

func TestNew_DetectsMLflow(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("MLFLOW_RUN_ID", "abc123def456")
	t.Setenv("MLFLOW_EXPERIMENT_ID", "7")
	t.Setenv("MLFLOW_TRACKING_URI", "https://mlflow.example.com")

	c := New()

	if c.Runner != "mlflow" {
		t.Errorf("Runner = %q, want mlflow", c.Runner)
	}
	if c.RunID != "abc123def456" {
		t.Errorf("RunID = %q, want abc123def456", c.RunID)
	}
	wantLabels := map[string]string{
		"experiment_id": "7",
		"tracking_uri":  "https://mlflow.example.com",
	}
	for k, v := range wantLabels {
		if got := c.Labels[k]; got != v {
			t.Errorf("Labels[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestNew_MLflowSkipsTrackingUriWhenLocalPath(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("MLFLOW_RUN_ID", "run-99")
	t.Setenv("MLFLOW_TRACKING_URI", "./mlruns")

	c := New()

	if c.Runner != "mlflow" {
		t.Errorf("Runner = %q, want mlflow", c.Runner)
	}
	if _, ok := c.Labels["tracking_uri"]; ok {
		t.Errorf("tracking_uri label should be skipped for local-file URI, got %q", c.Labels["tracking_uri"])
	}
}

func TestNew_MLflowWinsOverKubernetes(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("POD_NAME", "training-job-pod")
	t.Setenv("POD_NAMESPACE", "ml-experiments")
	t.Setenv("MLFLOW_RUN_ID", "abc123")
	t.Setenv("MLFLOW_EXPERIMENT_ID", "7")

	c := New()

	if c.Runner != "mlflow" {
		t.Errorf("Runner = %q, want mlflow (MLflow must win over K8s)", c.Runner)
	}
	if c.RunID != "abc123" {
		t.Errorf("RunID = %q, want abc123", c.RunID)
	}
	if _, ok := c.Labels["namespace"]; ok {
		t.Errorf("namespace leaked from K8s detection — MLflow should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["pod"]; ok {
		t.Errorf("pod leaked from K8s detection — MLflow should win cleanly: %#v", c.Labels)
	}
}

func TestNew_CIDetectionWinsOverMLflow(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("MLFLOW_RUN_ID", "abc123")
	t.Setenv("MLFLOW_EXPERIMENT_ID", "7")
	t.Setenv("MLFLOW_TRACKING_URI", "https://mlflow.example.com")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "555")
	t.Setenv("GITHUB_REPOSITORY", "ding-labs/ding")

	c := New()

	if c.Runner != "github-actions" {
		t.Errorf("Runner = %q, want github-actions (CI must win over MLflow)", c.Runner)
	}
	if c.RunID != "555" {
		t.Errorf("RunID = %q, want 555", c.RunID)
	}
	if _, ok := c.Labels["experiment_id"]; ok {
		t.Errorf("experiment_id leaked from MLflow — CI should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["tracking_uri"]; ok {
		t.Errorf("tracking_uri leaked from MLflow — CI should win cleanly: %#v", c.Labels)
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

func TestNew_DetectsArgoWorkflows(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("ARGO_TEMPLATE", `{"name":"main"}`)
	t.Setenv("ARGO_WORKFLOW_UID", "wf-uid-123")
	t.Setenv("ARGO_WORKFLOW_NAME", "dingdemo-abc12")
	t.Setenv("ARGO_NODE_ID", "dingdemo-abc12-train-7654321")
	t.Setenv("ARGO_POD_NAME", "dingdemo-abc12-train-7654321")
	t.Setenv("POD_NAMESPACE", "ml-experiments")

	c := New()

	if c.Runner != "argo-workflows" {
		t.Errorf("Runner = %q, want argo-workflows", c.Runner)
	}
	if c.RunID != "wf-uid-123" {
		t.Errorf("RunID = %q, want wf-uid-123 (ARGO_WORKFLOW_UID)", c.RunID)
	}
	wantLabels := map[string]string{
		"workflow":  "dingdemo-abc12",
		"node":      "dingdemo-abc12-train-7654321",
		"pod":       "dingdemo-abc12-train-7654321",
		"namespace": "ml-experiments",
	}
	for k, v := range wantLabels {
		if got := c.Labels[k]; got != v {
			t.Errorf("Labels[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestNew_ArgoFallsBackToWorkflowNameWhenNoUID(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("ARGO_TEMPLATE", `{}`)
	t.Setenv("ARGO_WORKFLOW_NAME", "dingdemo-abc12")
	// ARGO_WORKFLOW_UID intentionally unset — exercise fallback.

	c := New()

	if c.Runner != "argo-workflows" {
		t.Errorf("Runner = %q, want argo-workflows", c.Runner)
	}
	if c.RunID != "dingdemo-abc12" {
		t.Errorf("RunID = %q, want dingdemo-abc12 (ARGO_WORKFLOW_NAME fallback)", c.RunID)
	}
}

// TestNew_ArgoWinsOverKubernetes locks the ordering rule that Argo
// detection takes precedence over bare Kubernetes detection. An Argo
// step's pod sets BOTH ARGO_TEMPLATE and KUBERNETES_SERVICE_HOST;
// Argo's labels are richer for alerting, so it must win.
func TestNew_ArgoWinsOverKubernetes(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("POD_NAME", "wf-pod-xyz")
	t.Setenv("POD_NAMESPACE", "argo")
	t.Setenv("NODE_NAME", "ip-10-0-1-100.ec2.internal")
	t.Setenv("ARGO_TEMPLATE", `{}`)
	t.Setenv("ARGO_WORKFLOW_UID", "wf-uid")
	t.Setenv("ARGO_WORKFLOW_NAME", "wf-name")

	c := New()

	if c.Runner != "argo-workflows" {
		t.Errorf("Runner = %q, want argo-workflows (Argo must win over K8s)", c.Runner)
	}
	if c.RunID != "wf-uid" {
		t.Errorf("RunID = %q, want wf-uid", c.RunID)
	}
	if _, ok := c.Labels["node"]; ok {
		t.Errorf("node leaked from K8s detection — Argo should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["job_name"]; ok {
		t.Errorf("job_name leaked from K8s detection — Argo should win cleanly: %#v", c.Labels)
	}
}

// Locks ordering so a future reorder of the detect() switch is caught;
// parallels TestNew_CIDetectionWinsOverKubernetes's rationale comment.
func TestNew_ArgoWinsOverMLflow(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("MLFLOW_RUN_ID", "mlflow-run-abc")
	t.Setenv("MLFLOW_EXPERIMENT_ID", "7")
	t.Setenv("MLFLOW_TRACKING_URI", "https://mlflow.example.com")
	t.Setenv("ARGO_TEMPLATE", `{}`)
	t.Setenv("ARGO_WORKFLOW_UID", "wf-uid")
	t.Setenv("ARGO_WORKFLOW_NAME", "wf-name")

	c := New()

	if c.Runner != "argo-workflows" {
		t.Errorf("Runner = %q, want argo-workflows (Argo must win over MLflow)", c.Runner)
	}
	if c.RunID != "wf-uid" {
		t.Errorf("RunID = %q, want wf-uid", c.RunID)
	}
	if _, ok := c.Labels["experiment_id"]; ok {
		t.Errorf("experiment_id leaked from MLflow — Argo should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["tracking_uri"]; ok {
		t.Errorf("tracking_uri leaked from MLflow — Argo should win cleanly: %#v", c.Labels)
	}
}

func TestNew_CIDetectionWinsOverArgo(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("ARGO_TEMPLATE", `{}`)
	t.Setenv("ARGO_WORKFLOW_UID", "wf-uid")
	t.Setenv("ARGO_WORKFLOW_NAME", "wf-name")
	t.Setenv("ARGO_NODE_ID", "node-id")
	t.Setenv("ARGO_POD_NAME", "wf-pod")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "555")
	t.Setenv("GITHUB_REPOSITORY", "ding-labs/ding")

	c := New()

	if c.Runner != "github-actions" {
		t.Errorf("Runner = %q, want github-actions (CI must win over Argo)", c.Runner)
	}
	if c.RunID != "555" {
		t.Errorf("RunID = %q, want 555", c.RunID)
	}
	if _, ok := c.Labels["workflow"]; ok {
		t.Errorf("workflow leaked from Argo — CI should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["node"]; ok {
		t.Errorf("node leaked from Argo — CI should win cleanly: %#v", c.Labels)
	}
}

func TestNew_DetectsRay(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("RAY_JOB_ID", "raysubmit_abcdef1234567890")

	c := New()

	if c.Runner != "ray" {
		t.Errorf("Runner = %q, want ray", c.Runner)
	}
	if c.RunID != "raysubmit_abcdef1234567890" {
		t.Errorf("RunID = %q, want raysubmit_abcdef1234567890 (RAY_JOB_ID)", c.RunID)
	}
}

// TestNew_RayWinsOverKubernetes locks the ordering rule that Ray
// detection takes precedence over bare Kubernetes detection. A Ray
// job submitted to a KubeRay cluster sets BOTH RAY_JOB_ID and
// KUBERNETES_SERVICE_HOST; Ray's labels are richer for alerting.
func TestNew_RayWinsOverKubernetes(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("POD_NAME", "ray-head-pod")
	t.Setenv("POD_NAMESPACE", "ray-system")
	t.Setenv("NODE_NAME", "ip-10-0-1-100.ec2.internal")
	t.Setenv("RAY_JOB_ID", "raysubmit_abc")

	c := New()

	if c.Runner != "ray" {
		t.Errorf("Runner = %q, want ray (Ray must win over K8s)", c.Runner)
	}
	if c.RunID != "raysubmit_abc" {
		t.Errorf("RunID = %q, want raysubmit_abc", c.RunID)
	}
	if _, ok := c.Labels["namespace"]; ok {
		t.Errorf("namespace leaked from K8s detection — Ray should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["pod"]; ok {
		t.Errorf("pod leaked from K8s detection — Ray should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["job_name"]; ok {
		t.Errorf("job_name leaked from K8s detection — Ray should win cleanly: %#v", c.Labels)
	}
}

// TestNew_RayWinsOverMLflow locks ordering: a Ray job that uses MLflow
// internally for tracking should report as Ray — the orchestrator is the
// more useful label for alerting; MLflow is the experiment-tracking layer
// underneath.
func TestNew_RayWinsOverMLflow(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("MLFLOW_RUN_ID", "mlflow-run-abc")
	t.Setenv("MLFLOW_EXPERIMENT_ID", "7")
	t.Setenv("MLFLOW_TRACKING_URI", "https://mlflow.example.com")
	t.Setenv("RAY_JOB_ID", "raysubmit_abc")

	c := New()

	if c.Runner != "ray" {
		t.Errorf("Runner = %q, want ray (Ray must win over MLflow)", c.Runner)
	}
	if c.RunID != "raysubmit_abc" {
		t.Errorf("RunID = %q, want raysubmit_abc", c.RunID)
	}
	if _, ok := c.Labels["experiment_id"]; ok {
		t.Errorf("experiment_id leaked from MLflow — Ray should win cleanly: %#v", c.Labels)
	}
	if _, ok := c.Labels["tracking_uri"]; ok {
		t.Errorf("tracking_uri leaked from MLflow — Ray should win cleanly: %#v", c.Labels)
	}
}

func TestNew_CIDetectionWinsOverRay(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("RAY_JOB_ID", "raysubmit_abc")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "555")
	t.Setenv("GITHUB_REPOSITORY", "ding-labs/ding")

	c := New()

	if c.Runner != "github-actions" {
		t.Errorf("Runner = %q, want github-actions (CI must win over Ray)", c.Runner)
	}
	if c.RunID != "555" {
		t.Errorf("RunID = %q, want 555", c.RunID)
	}
	if c.RunID == "raysubmit_abc" {
		t.Errorf("RunID leaked from Ray — CI should win cleanly")
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
		"MLFLOW_RUN_ID", "MLFLOW_EXPERIMENT_ID", "MLFLOW_TRACKING_URI",
		"ARGO_TEMPLATE", "ARGO_WORKFLOW_UID", "ARGO_WORKFLOW_NAME", "ARGO_NODE_ID", "ARGO_POD_NAME",
		"RAY_JOB_ID",
	} {
		t.Setenv(k, "")
	}
}
