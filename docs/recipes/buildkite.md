# Using DING with Buildkite

> Buildkite is a hosted CI control plane that runs jobs on your own infrastructure. DING's `ding run` wraps a step, evaluates rules during the step, and fires alerts on exit. Buildkite's `buildkite-agent annotate` API would let DING surface alerts directly into the build UI — see escalation criteria below for the Tier-2 path.

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- A Buildkite organization with at least one agent ([free trial available](https://buildkite.com/pricing))
- A notifier endpoint (Slack webhook URL, custom webhook, etc.)

## Minimal example

`.buildkite/pipeline.yml`:

```yaml
steps:
  - label: "Test with DING"
    command: |
      curl -sSL https://github.com/ding-labs/ding/releases/latest/download/ding_linux_amd64.tar.gz | tar -xz
      ./ding run --config ding.yaml -- ./run-tests.sh
```

`ding.yaml`:

```yaml
notifiers:
  slack:
    type: slack
    url: ${SLACK_WEBHOOK_URL}

rules:
  - name: ci_step_failed
    match:
      metric: run.exit
    condition: value > 0
    message: "{{ .repo }}@{{ .branch }} failed (exit {{ .exit_code }})"
    alert:
      - notifier: slack
```

Set `SLACK_WEBHOOK_URL` as an [environment hook](https://buildkite.com/docs/agent/v3/hooks) on your agent or as a pipeline-level environment variable.

## What you get

A Slack message when the step exits non-zero, automatically tagged with `repo`, `branch`, `commit` from Buildkite's environment. Successful steps produce no notification.

## Configuration

`runctx` auto-detects Buildkite via the `BUILDKITE=true` environment variable and captures these labels:

| Label | Buildkite env var |
|---|---|
| `run_id` | `BUILDKITE_BUILD_ID` |
| `runner` | `"buildkite"` (set by runctx) |
| `repo` | `BUILDKITE_PIPELINE_SLUG` |
| `branch` | `BUILDKITE_BRANCH` |
| `commit` | `BUILDKITE_COMMIT` |

Use these in `match.labels` or `message` templates. See [Configuration](../configuration.md) for the full notifier reference.

## Verification

1. Locally: `ding validate --config ding.yaml` — confirms the rule parses.
2. Trigger a build. Confirm a successful step produces no alert.
3. Force a failure (`exit 1` in `run-tests.sh`). Confirm the alert fires in Slack within ~5 seconds of step exit.

If the alert doesn't fire, check the Buildkite build log for `ding` output. Common issues: webhook URL not exposed (env hook scope, agent vs pipeline level), or `drain_timeout` shorter than the notifier retry window — see [Configuration](../configuration.md).

## Tradeoffs / known limitations

- **No `job` label by default.** runctx captures Buildkite's pipeline-level identifiers but not step-level (`BUILDKITE_STEP_KEY`). Add explicit `match.labels` if you need per-step rules.
- **Binary download per step.** Bake DING into your agent image, or use a [`pre-command` hook](https://buildkite.com/docs/agent/v3/hooks#available-hooks) to install it once per agent.
- **Annotation surface unused.** Buildkite has `buildkite-agent annotate`, the analogue of GHA's `$GITHUB_STEP_SUMMARY`. The minimal recipe doesn't use it; surfacing alerts back into the build UI would be a Tier-2 abstraction (`type: buildkite_annotate` notifier).

## Escalation criteria

This recipe is **a Tier-2 candidate** by the program's standard rubric:

- **Setup commands required:** 1 (`curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~24 — under threshold of 50
- **"Gotcha" callouts:** 3 (no `job` label, binary download, no annotation surface) — over threshold of 2 → **Tier-2 candidate**
- **End-to-end runnable:** yes (Buildkite has a free trial; the underlying agent is OSS and self-hostable indefinitely)

**Tier-2 candidate.** The structural friction is "annotations not used" — Buildkite users expect alerts to land in the build UI, not just Slack. A `type: buildkite_annotate` notifier (calling `buildkite-agent annotate --style error --context ding`) is the natural Tier-2 abstraction. Sequence it after GitLab CI's artifact notifier (similar shape).
