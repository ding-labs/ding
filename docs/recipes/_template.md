# Using DING with <Platform>

<!--
This is the canonical template every recipe in docs/recipes/ must conform to.
When writing a new recipe: copy this file, rename it `<platform>.md` (lowercase,
hyphens), and replace the placeholders. Keep the section structure verbatim —
the program's value comes from every recipe being judgeable against the same
rubric.
-->

> **Two-sentence framing.** What is this platform? Why does co-mortal observability fit?

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- <Platform-specific requirements: account, runtime version, etc.>
- A notifier endpoint (Slack webhook URL, custom webhook, etc.)

## Minimal example

The shortest config that produces a working alert when the workload exits non-zero.

```yaml
# ding.yaml
notifiers:
  slack:
    type: slack
    url: $SLACK_WEBHOOK_URL

rules:
  - name: job_failed
    match:
      metric: run.exit
    condition: value > 0
    message: "Job failed (exit {{ .exit_code }})"
    alert:
      - notifier: slack
```

```yaml
# Platform-specific config — wrap your job command in `ding run`
# <platform-specific YAML / shell snippet here>
```

## What you get

A textual or screenshot rendering of the alert as actually delivered. Include the auto-captured labels visible in the alert.

## Configuration

`runctx` auto-detects this platform and captures these labels:

| Label | Env var on the platform |
|---|---|
| `run_id` | `<env var>` |
| `runner` | `"<platform-slug>"` (set by runctx) |
| `repo` | `<env var>` |
| `branch` | `<env var>` |
| `commit` | `<env var>` |
| `job` | `<env var>` |

Use these as label keys in `match.labels` or `message` template variables.

## Verification

1. Run `ding validate --config ding.yaml` locally to confirm the rule parses.
2. Trigger a job that exits 0 — confirm no alert fires.
3. Trigger a job that exits non-zero — confirm the alert fires within `drain_timeout` of job exit.

## Tradeoffs / known limitations

- <Honest list of what the recipe doesn't solve.>
- <Especially: anything the platform forces a workaround for.>

## Escalation criteria

This recipe is **<Tier 1 | a Tier 2 candidate>** by the program's standard rubric:

- **Setup commands required:** `<N>` — <under / over> threshold of 5
- **Boilerplate lines:** `<N>` — <under / over> threshold of 50
- **"Gotcha" callouts:** `<N>` — <under / over> threshold of 2
- **End-to-end runnable:** <yes / no — reason>

<If Tier 2 candidate: one-paragraph note on what abstraction (`ding-<platform>` repo, Helm chart, plugin, etc.) would collapse the friction.>
