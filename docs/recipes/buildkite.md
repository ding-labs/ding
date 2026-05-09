# Using DING with Buildkite

> Buildkite is a hosted CI control plane that runs jobs on your own infrastructure. DING's `ding run` wraps a step, evaluates rules during the step, and fires alerts on exit. Buildkite's `buildkite-agent annotate` API would let DING surface alerts directly into the build UI — see escalation criteria below for the Tier-2 path.

## Prerequisites

- DING binary `>= v0.10.0` — see [install](../install.md)
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
    message: "{{ .repo }}@{{ .branch }} failed (exit {{ .exit_code }} after {{ .duration_seconds }}s)"
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

If the alert doesn't fire, check the Buildkite build log for `ding` output. Common issues: webhook URL not exposed (env hook scope, agent vs pipeline level), or `drain_timeout` shorter than the notifier retry window — see [Configuration → drain_timeout](../configuration.md#drain_timeout-and-retry-behaviour-in-ding-run).

## Native Buildkite UI surfacing

If you want DING alerts to surface as Buildkite build annotations (visible at the top of the build UI) rather than (or alongside) external notifiers, DING ships a built-in `type: buildkite_annotate` notifier. See [`type: buildkite_annotate`](../configuration.md#type-buildkite_annotate) for the full reference.

Add to `ding.yaml`:

```yaml
notifiers:
  annotate:
    type: buildkite_annotate
    # style: error  # default; success | info | warning | error
```

No changes to `.buildkite/pipeline.yml` are needed — the `buildkite-agent` CLI is already on PATH inside Buildkite jobs. All DING alerts from a build land in a single rolling annotation (`--context ding --append`) so the UI stays uncluttered. Outside Buildkite (e.g. local dev), the notifier no-ops gracefully after a one-time warning.

## Tradeoffs / known limitations

- **No `job` label by default.** runctx captures Buildkite's pipeline-level identifiers but not step-level (`BUILDKITE_STEP_KEY`). Add explicit `match.labels` if you need per-step rules.
- **Binary download per step.** Bake DING into your agent image, or use a [`pre-command` hook](https://buildkite.com/docs/agent/v3/hooks#available-hooks) to install it once per agent.

## Escalation criteria

This recipe is **a Tier-2 candidate** by the program's standard rubric:

- **Setup commands required:** 1 (`curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~24 — under threshold of 50
- **"Gotcha" callouts:** 2 (no `job` label, binary download) — at threshold of 2
- **End-to-end runnable:** yes (Buildkite has a free trial; the underlying agent is OSS and self-hostable indefinitely)

Buildkite-native alert surfacing now ships as the built-in [`type: buildkite_annotate`](../configuration.md#type-buildkite_annotate) notifier (covered in the section above). The remaining gotchas (no `job` label, binary download per step) are environmental rather than implementation gaps; the recipe stays Tier 1.
