# Using DING with GitLab CI

> GitLab CI is GitLab's native CI/CD runner. DING's `ding run` subcommand wraps a CI job, captures its output, evaluates rules during the run, and fires alerts when the job exits — without any backend infrastructure.

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- A GitLab project with CI enabled (gitlab.com or self-hosted)
- A notifier endpoint (Slack webhook URL, custom webhook, etc.) accessible from the runner

## Minimal example

`.gitlab-ci.yml`:

```yaml
test_with_ding:
  image: alpine:latest
  before_script:
    - apk add --no-cache curl tar
    - curl -sSL https://github.com/ding-labs/ding/releases/latest/download/ding_linux_amd64.tar.gz | tar -xz
  script:
    - ./ding run --config ding.yaml -- ./run-tests.sh
```

`ding.yaml` (committed alongside `.gitlab-ci.yml`):

```yaml
notifiers:
  slack:
    type: slack
    url: ${SLACK_WEBHOOK_URL}

rules:
  - name: ci_job_failed
    match:
      metric: run.exit
    condition: value > 0
    message: "Pipeline {{ .branch }} failed (exit {{ .exit_code }})"
    alert:
      - notifier: slack
```

Set `SLACK_WEBHOOK_URL` as a [protected CI/CD variable](https://docs.gitlab.com/ci/variables/) in your project settings.

## What you get

A Slack message when the pipeline exits non-zero, automatically tagged with `repo`, `branch`, `commit`, `job` pulled from GitLab's environment. Successful runs produce no notification.

## Configuration

`runctx` auto-detects GitLab CI via the `GITLAB_CI=true` environment variable and captures these labels:

| Label | GitLab env var |
|---|---|
| `run_id` | `CI_PIPELINE_ID` |
| `runner` | `"gitlab-ci"` (set by runctx) |
| `repo` | `CI_PROJECT_PATH` |
| `branch` | `CI_COMMIT_REF_NAME` |
| `commit` | `CI_COMMIT_SHA` |
| `job` | `CI_JOB_NAME` |

Use these in `match.labels` for selective rules, or in `message` templates as `{{ .branch }}`, `{{ .commit }}`, etc. See [Configuration](../configuration.md) for the full notifier reference.

## Verification

1. Locally: `ding validate --config ding.yaml` — confirms the rule parses.
2. Push a commit. Confirm the pipeline runs and that a successful job produces no alert.
3. Force a failure: change `run-tests.sh` to `exit 1`. Confirm the alert fires in Slack within ~5 seconds of job exit.

If the alert doesn't fire, check the GitLab CI job log for `ding` output. Common issues: webhook URL not exposed to the job (mark the variable as not "Protected" if testing on a non-protected branch), or `drain_timeout` shorter than the notifier retry window — see [Configuration → drain_timeout](../configuration.md).

## Tradeoffs / known limitations

- **No native step-summary surface.** GitHub Actions has `$GITHUB_STEP_SUMMARY`; GitLab does not. Alerts go to your notifier of choice (Slack, webhook, etc.), not into the GitLab UI itself. Surfacing alerts back into GitLab would require a future Tier-2 abstraction (an artifact-writing notifier) — see escalation criteria below.
- **Binary download per job.** The minimal example downloads DING from GitHub Releases each run (~4MB tarball, plus an `apk add curl tar` round-trip on `alpine:latest` — typically 5–10s of preamble cold). For high-frequency pipelines, bake DING and its dependencies into your CI image instead.

## Escalation criteria

This recipe is **Tier 1** by the program's standard rubric:

- **Setup commands required:** 2 (`apk add`, `curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~24 across `.gitlab-ci.yml` and `ding.yaml` — under threshold of 50
- **"Gotcha" callouts:** 2 (no step-summary surface, binary download per job) — at threshold of 2
- **End-to-end runnable:** yes (gitlab.com has a free tier sufficient for evaluation; minutes allotment varies — see [GitLab pricing](https://about.gitlab.com/pricing/))

The "no step-summary surface" callout is the only structural friction. If users start asking for GitLab-native alert surfacing, that's the trigger to promote this to Tier 2 with an artifact-writing notifier (`type: gitlab_artifact` or similar).
