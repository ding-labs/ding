// Package runctx holds run/job-scoped metadata for `ding run` mode.
//
// A Context auto-detects the surrounding CI/job runner (GitHub Actions,
// GitLab CI, CircleCI, Jenkins) from environment variables and exposes
// helpers that attach run-scoped labels to events flowing through the
// alerting engine. On run exit, SummaryEvent produces a synthetic
// "run.exit" event with the exit code and run duration so rules can
// match on job-level outcomes.
package runctx

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"time"

	"github.com/zuchka/ding/internal/ingester"
)

// Context is the per-run metadata bag.
type Context struct {
	RunID     string
	Runner    string
	StartedAt time.Time
	Labels    map[string]string

	ExitCode *int
	EndedAt  *time.Time
}

// New constructs a Context, auto-detecting the runner from env.
// If no CI runner is detected, Runner is "local" and RunID is a random hex.
func New() *Context {
	c := &Context{
		StartedAt: time.Now(),
		Labels:    map[string]string{},
	}
	c.detect()
	if c.RunID == "" {
		c.RunID = generateID()
	}
	if c.Runner == "" {
		c.Runner = "local"
	}
	return c
}

// detect populates Runner, RunID, and standard labels from environment.
// Order matters: the first matching runner wins. Each branch only sets fields
// that are present, so missing env vars don't pollute Labels with empty values.
func (c *Context) detect() {
	switch {
	case os.Getenv("GITHUB_ACTIONS") == "true":
		c.Runner = "github-actions"
		c.RunID = os.Getenv("GITHUB_RUN_ID")
		setIf(c.Labels, "repo", os.Getenv("GITHUB_REPOSITORY"))
		setIf(c.Labels, "branch", os.Getenv("GITHUB_REF_NAME"))
		setIf(c.Labels, "commit", os.Getenv("GITHUB_SHA"))
		setIf(c.Labels, "workflow", os.Getenv("GITHUB_WORKFLOW"))
		setIf(c.Labels, "job", os.Getenv("GITHUB_JOB"))
		setIf(c.Labels, "actor", os.Getenv("GITHUB_ACTOR"))
		setIf(c.Labels, "event", os.Getenv("GITHUB_EVENT_NAME"))
	case os.Getenv("GITLAB_CI") == "true":
		c.Runner = "gitlab-ci"
		c.RunID = os.Getenv("CI_PIPELINE_ID")
		setIf(c.Labels, "repo", os.Getenv("CI_PROJECT_PATH"))
		setIf(c.Labels, "branch", os.Getenv("CI_COMMIT_REF_NAME"))
		setIf(c.Labels, "commit", os.Getenv("CI_COMMIT_SHA"))
		setIf(c.Labels, "job", os.Getenv("CI_JOB_NAME"))
	case os.Getenv("CIRCLECI") == "true":
		c.Runner = "circleci"
		c.RunID = os.Getenv("CIRCLE_BUILD_NUM")
		setIf(c.Labels, "repo", os.Getenv("CIRCLE_PROJECT_REPONAME"))
		setIf(c.Labels, "branch", os.Getenv("CIRCLE_BRANCH"))
		setIf(c.Labels, "commit", os.Getenv("CIRCLE_SHA1"))
		setIf(c.Labels, "job", os.Getenv("CIRCLE_JOB"))
	case os.Getenv("JENKINS_URL") != "":
		c.Runner = "jenkins"
		c.RunID = os.Getenv("BUILD_TAG")
		setIf(c.Labels, "job", os.Getenv("JOB_NAME"))
		setIf(c.Labels, "build", os.Getenv("BUILD_NUMBER"))
	case os.Getenv("BUILDKITE") == "true":
		c.Runner = "buildkite"
		c.RunID = os.Getenv("BUILDKITE_BUILD_ID")
		setIf(c.Labels, "repo", os.Getenv("BUILDKITE_PIPELINE_SLUG"))
		setIf(c.Labels, "branch", os.Getenv("BUILDKITE_BRANCH"))
		setIf(c.Labels, "commit", os.Getenv("BUILDKITE_COMMIT"))
	}
}

// Apply merges run-scoped labels into the supplied label map, returning the
// (possibly newly-allocated) result. Existing keys on the input win — the
// run context never clobbers user-supplied labels.
func (c *Context) Apply(labels map[string]string) map[string]string {
	out := labels
	if out == nil {
		out = map[string]string{}
	}
	setIfMissing(out, "run_id", c.RunID)
	setIfMissing(out, "runner", c.Runner)
	for k, v := range c.Labels {
		setIfMissing(out, k, v)
	}
	return out
}

// SummaryEvent returns the synthetic event emitted at the end of a run.
// Metric is "run.exit"; Value is the exit code as float; duration_seconds
// and exit_code appear in Floats; run-scoped labels are attached.
// Mutates c to record ExitCode and EndedAt.
func (c *Context) SummaryEvent(exitCode int) ingester.Event {
	now := time.Now()
	c.ExitCode = &exitCode
	c.EndedAt = &now
	dur := now.Sub(c.StartedAt).Seconds()

	labels := c.Apply(map[string]string{})
	labels["exit_code"] = strconv.Itoa(exitCode)

	return ingester.Event{
		Metric: "run.exit",
		Value:  float64(exitCode),
		Labels: labels,
		Floats: map[string]float64{
			"exit_code":        float64(exitCode),
			"duration_seconds": dur,
		},
		At: now,
	}
}

func setIf(m map[string]string, k, v string) {
	if v != "" {
		m[k] = v
	}
}

func setIfMissing(m map[string]string, k, v string) {
	if v == "" {
		return
	}
	if _, ok := m[k]; ok {
		return
	}
	m[k] = v
}

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}
