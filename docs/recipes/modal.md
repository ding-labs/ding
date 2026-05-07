# Using DING with Modal

> Modal is serverless container compute popular for ML — `@app.function`-decorated Python runs in containers Modal builds and schedules on its infrastructure. DING wraps `modal run` *locally*, ingests JSON events streamed back from the remote function via Modal's stdout-forwarding, and fires alerts on metric thresholds and exit code — both during the run *and* on exit, with alerts auto-tagged by labels the function emits.

## Prerequisites

- DING binary `>= v0.8.0` — see [install](../install.md)
- Modal CLI (`pip install modal`) authenticated via `modal token new`
- A Modal account (free tier with $30/mo credit covers this recipe end-to-end)
- A notifier endpoint (Slack webhook URL is the canonical example)

## Minimal example

The shortest config that wraps a Modal function so alerts fire during execution and on function exit.

`app.py`:

```python
import json, os, sys, modal

app = modal.App("ding-demo")
img = modal.Image.debian_slim().pip_install("numpy")  # or whatever your function needs

@app.function(image=img)
def trainer(epochs: int = 10):
    # Modal sets these env vars inside the container; surface them in events for alert labels.
    task_id = os.environ.get("MODAL_TASK_ID", "unknown")
    fn_name = os.environ.get("MODAL_FUNCTION_NAME", "trainer")

    for epoch in range(epochs):
        loss = compute_loss(epoch)
        # Flat top-level JSON keys — DING's ingester extracts strings as labels and numbers as floats.
        # Nested objects are skipped, so don't wrap labels in a {"labels": {...}} sub-object.
        print(json.dumps({
            "metric": "val_loss",
            "value": loss,
            "epoch": str(epoch),
            "modal_task_id": task_id,
            "function_name": fn_name,
        }), flush=True)

    if some_failure_condition:
        sys.exit(1)
```

`ding.yaml`:

```yaml
notifiers:
  slack:
    type: slack
    url: ${SLACK_WEBHOOK_URL}

rules:
  # During-run: fire if validation loss spikes mid-training.
  - name: loss_spike
    match: { metric: val_loss }
    condition: value > 10
    cooldown: 1m
    message: "val_loss spike: {{ .value }} on epoch {{ .epoch }} (Modal task {{ .modal_task_id }})"
    alert:
      - notifier: slack

  # Default mode (during-run): fire if the wrapped command exits non-zero.
  # The synthetic run.exit event is dispatched at end-of-run; this rule fires
  # once when the wrapped command exits.
  - name: training_failed
    match: { metric: run.exit }
    condition: value > 0
    message: "Modal function {{ .function_name }} (task {{ .modal_task_id }}) failed (exit {{ .exit_code }})"
    alert:
      - notifier: slack
```

Invoke:

```bash
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
ding run --config ding.yaml -- modal run app.py::trainer
```

DING runs locally, wrapping the `modal run` CLI. The Modal function's `print` output streams back through Modal's CLI to DING's stdin; DING parses the JSON events, applies rules, and fires Slack alerts. When `modal run` exits, DING dispatches the `run.exit` synthetic event so end-of-run rules can fire, then exits with the function's exit code.

## What you get

A Slack message during training when `val_loss` exceeds threshold:

> 🔔 `loss_spike`
> val_loss spike: 12.4 on epoch 7 (Modal task ta-abc123def)

…and on function exit:

> 🔔 `training_failed`
> Modal function trainer (task ta-abc123def) failed (exit 1)

The `modal_task_id` matches the task ID visible in the Modal dashboard, so the Slack alert is one click away from the function's logs and metrics.

## Configuration

DING does **not** auto-detect Modal — Modal's runtime owns the container entrypoint, so `ding run --` cannot wrap *inside* the container. Instead, the function emits Modal context as JSON event labels. Modal sets these env vars inside the container that the function can read:

| Env var | Purpose |
|---|---|
| `MODAL_TASK_ID` | Per-invocation task ID (matches Modal dashboard) |
| `MODAL_FUNCTION_NAME` | The function's Python name |
| `MODAL_APP_NAME` | The Modal `App` name (for multi-function apps) |

Emit any subset as flat top-level JSON keys (DING's ingester at `internal/ingester/json.go` extracts top-level strings as labels and numbers as floats; nested objects are skipped). Use them in `match.labels` or `message` template variables. See [Configuration](../configuration.md) for the full reference.

## Verification

```bash
pip install modal
modal token new                                # opens browser; one-time
mkdir modal-smoke && cd modal-smoke
# Author app.py + ding.yaml from the example above; have trainer() print val_loss=12.4 then sys.exit(1).
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
ding run --config ding.yaml -- modal run app.py::trainer
# Verify in Slack:
#   1. loss_spike alert delivered (val_loss > 10) mid-execution
#   2. training_failed alert delivered with exit code 1 within ~5s of function exit
#   3. modal_task_id label matches Modal dashboard's task ID for this invocation
```

If the alert doesn't fire, check the Modal dashboard for the function's stdout — the JSON events should be visible. Common issues: missing `flush=True` in `print` (Modal buffers stdout otherwise), or `SLACK_WEBHOOK_URL` not exported in the local shell.

## Tradeoffs / known limitations

- **No runctx auto-detection.** Modal's runtime owns the container entrypoint, so `ding run --` can't wrap inside the container the way it does in Kubernetes/Argo/Ray. DING runs locally and ingests events from `modal run`'s stdout stream. Users emit Modal labels manually as flat-key JSON.
- **`modal deploy` long-lived apps not covered.** Deployed apps invoked via `Function.from_name(...).remote()` are service-shaped, not wedge-shaped — `modal run` is the only invocation pattern this recipe covers. For long-running Modal services, the `ding serve` daemon is the canonical answer, not this recipe.
- **Async `.spawn()` invocations.** DING wraps the synchronous `modal run` call; async-spawned tasks return immediately on the local side. For async fan-out alerting, the spawned function itself must emit alerts inline (not via `run.exit`).
- **Slack webhook stays local.** `SLACK_WEBHOOK_URL` lives in the laptop's shell; DING fires alerts from the local process. Don't forward it to Modal as a `Secret` — the function never needs it. (Different from Ray's `--runtime-env-json` shape, where the webhook does cross.)

## Escalation criteria

This recipe is **Tier 1** by the program's standard rubric, with a Tier-2 promotion candidate flagged:

- **Setup commands required:** 2 (`pip install modal`, `modal token new`) — under threshold of 5
- **Boilerplate lines:** ~40 — under threshold of 50
- **"Gotcha" callouts:** 4 — over threshold of 2 → **promotion trigger**
- **End-to-end runnable:** yes (Modal $30/mo free credit covers this trivially)

A future Tier-2 candidate worth tracking: **`pip install ding-modal` Python helper** — provides a decorator (e.g., `@ding.modal_function()`) wrapping `@app.function` that auto-emits Modal task metadata (`MODAL_TASK_ID`, `MODAL_FUNCTION_NAME`, `MODAL_APP_NAME`) as flat-key JSON events alongside DING events. Removes the manual `print(json.dumps(...))` boilerplate and the env-var lookup in user code. Separate repo (parallel to `ding-action`), pip-installable. Not built here.
