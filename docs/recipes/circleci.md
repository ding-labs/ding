# Using DING with CircleCI

> CircleCI is a hosted CI/CD platform built around YAML-defined workflows. DING's `ding run` wraps a job step, evaluates rules, and fires alerts on exit — leveraging CircleCI's environment to auto-tag every alert with project, branch, and commit context.

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- A CircleCI project (free tier covers most personal/OSS use)
- A notifier endpoint (Slack webhook URL, custom webhook, etc.)

## Minimal example

`.circleci/config.yml`:

```yaml
version: 2.1

jobs:
  test:
    docker:
      - image: cimg/base:current
    steps:
      - checkout
      - run:
          name: Install DING
          command: |
            curl -sSL https://github.com/ding-labs/ding/releases/latest/download/ding_linux_amd64.tar.gz | tar -xz
      - run:
          name: Run tests with DING
          command: ./ding run --config ding.yaml -- ./run-tests.sh

workflows:
  test_workflow:
    jobs:
      - test
```

`ding.yaml`:

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
    message: "{{ .repo }}@{{ .branch }} failed (exit {{ .exit_code }})"
    alert:
      - notifier: slack
```

Set `SLACK_WEBHOOK_URL` as an [environment variable](https://circleci.com/docs/env-vars/) in your CircleCI project settings.

## What you get

A Slack message when the job exits non-zero, automatically tagged with `repo`, `branch`, `commit`, `job` from CircleCI's environment. Successful runs produce no notification.

## Configuration

`runctx` auto-detects CircleCI via the `CIRCLECI=true` environment variable and captures these labels:

| Label | CircleCI env var |
|---|---|
| `run_id` | `CIRCLE_BUILD_NUM` |
| `runner` | `"circleci"` (set by runctx) |
| `repo` | `CIRCLE_PROJECT_REPONAME` |
| `branch` | `CIRCLE_BRANCH` |
| `commit` | `CIRCLE_SHA1` |
| `job` | `CIRCLE_JOB` |

Use these in `match.labels` or `message` templates. See [Configuration](../configuration.md) for the full notifier reference.

## Verification

1. Locally: `ding validate --config ding.yaml` — confirms the rule parses.
2. Push a commit. Confirm a successful job produces no alert.
3. Force a failure (`exit 1` in `run-tests.sh`). Confirm the alert fires in Slack within ~5 seconds of job exit.

If the alert doesn't fire, check the CircleCI job log for `ding` output. Common issues: webhook URL not exposed to the job (project-level vs context-level variable scoping), or `drain_timeout` shorter than the notifier retry window — see [Configuration](../configuration.md).

## Tradeoffs / known limitations

- **No native annotation surface.** CircleCI doesn't have a step-summary equivalent to GitHub Actions' `$GITHUB_STEP_SUMMARY`. Alerts go to your notifier; CircleCI's UI shows DING's stdout.
- **Binary download per job.** Bake DING into a [custom Docker image](https://circleci.com/docs/custom-images/) for high-frequency workflows.
- **Orb not provided.** A CircleCI orb (a packaged config wrapper) would collapse the install step into one line — that's the most likely Tier-2 promotion target.
- **Recipe assumes `cimg/base:current` includes `curl` and `tar`** (currently true). If you switch to a minimal custom image, add explicit install steps.

## Escalation criteria

This recipe is **Tier 1** by the program's standard rubric:

- **Setup commands required:** 1 (`curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~32 across `.circleci/config.yml` and `ding.yaml` — under threshold of 50
- **"Gotcha" callouts:** 3 structural (no annotation surface, binary download per job, no orb) — over threshold of 2 → **Tier-2 candidate**
- **End-to-end runnable:** yes (CircleCI free tier sufficient for evaluation; minutes allotment varies — see [CircleCI pricing](https://circleci.com/pricing/))

**Tier-2 candidate.** Three callouts cross the rubric threshold. The natural Tier-2 abstraction is a CircleCI orb (`ding-labs/ding`) that exposes a `ding/run` step — collapsing the install + invoke pattern into one line. Defer until 2+ users ask for it (per spec §"Open Questions" promotion authority).
