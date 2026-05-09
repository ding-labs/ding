# Using DING with GitLab CI

> GitLab CI is GitLab's native CI/CD runner. DING's `ding run` subcommand wraps a CI job, captures its output, evaluates rules during the run, and fires alerts when the job exits — without any backend infrastructure.

## Prerequisites

- DING binary `>= v0.10.0` — see [install](../install.md)
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
    message: "Pipeline {{ .branch }} failed (exit {{ .exit_code }} after {{ .duration_seconds }}s)"
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

If the alert doesn't fire, check the GitLab CI job log for `ding` output. Common issues: webhook URL not exposed to the job (mark the variable as not "Protected" if testing on a non-protected branch), or `drain_timeout` shorter than the notifier retry window — see [Configuration → drain_timeout](../configuration.md#drain_timeout-and-retry-behaviour-in-ding-run).

## Native GitLab UI surfacing

If you want DING alerts to surface as a GitLab pipeline artifact (downloadable from the job UI, browsable from the pipeline view) rather than (or alongside) external notifiers, DING ships a built-in `type: gitlab_artifact` notifier. See [`type: gitlab_artifact`](../configuration.md#type-gitlab_artifact) for the full reference.

Add to `ding.yaml`:

```yaml
notifiers:
  artifact:
    type: gitlab_artifact
    # path: ding-alerts.md  # default; relative to $CI_PROJECT_DIR
```

Add to `.gitlab-ci.yml`:

```yaml
test_with_ding:
  # ... existing job config ...
  artifacts:
    when: always       # archive even on job failure
    paths:
      - ding-alerts.md
```

After the pipeline runs, the file appears under the job's "Browse artifacts" link with the standard `# DING Alerts` Markdown header and one `## <rule>` section per fired alert.

## Tradeoffs / known limitations

- **Binary download per job.** The minimal example downloads DING from GitHub Releases each run (~4MB tarball, plus an `apk add curl tar` round-trip on `alpine:latest` — typically 5–10s of preamble cold). For high-frequency pipelines, bake DING and its dependencies into your CI image instead.

## Escalation criteria

This recipe is **Tier 1** by the program's standard rubric:

- **Setup commands required:** 2 (`apk add`, `curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~24 across `.gitlab-ci.yml` and `ding.yaml` — under threshold of 50
- **"Gotcha" callouts:** 1 (binary download per job) — under threshold of 2
- **End-to-end runnable:** yes (gitlab.com has a free tier sufficient for evaluation; minutes allotment varies — see [GitLab pricing](https://about.gitlab.com/pricing/))

GitLab-native alert surfacing now ships as the built-in [`type: gitlab_artifact`](../configuration.md#type-gitlab_artifact) notifier (covered in the section above). The recipe stays Tier 1 — no further Tier-2 promotion needed for GitLab CI.
