# Using DING with Ray

> Ray is the leading open-source distributed compute framework for ML — `ray.train` for distributed training, `ray.tune` for hyperparameter search, `ray.serve` for serving. DING's `ding run` wraps a Ray job's driver process, evaluates rules during the job, and fires alerts on metric thresholds and exit code — both during the job *and* on exit, with the alert auto-tagged with the Ray job ID.

## Prerequisites

- DING binary `>= v0.7.1` — see [install](../install.md)
- `ray >= 2.0` (`pip install "ray[default]"`; add `train`/`tune` extras as needed for your workload)
- A running Ray cluster: local single-node (`ray start --head`) for dev; KubeRay/Anyscale/EKS for production
- A notifier endpoint (Slack webhook URL is the canonical example)

## Minimal example

### Path A: `ray job submit` (auto-detected)

The recommended pattern. `RAY_JOB_ID` is set automatically by Ray when the job submission triggers the entry point command, so DING auto-tags every alert with `run_id` matching `ray job list` output.

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
    message: "val_loss spike: {{ .value }} on epoch {{ .epoch }} (Ray job {{ .run_id }})"
    alert:
      - notifier: slack

  # Default mode (during-run): fire if the training process exits non-zero.
  # The synthetic run.exit event is dispatched at end-of-run; this rule fires
  # once when the wrapped command exits.
  - name: training_failed
    match: { metric: run.exit }
    condition: value > 0
    message: "Ray job {{ .run_id }} failed (exit {{ .exit_code }})"
    alert:
      - notifier: slack
```

`train.py` (excerpt — emit JSON events for DING alongside Ray's native reporting):

```python
import json, ray
from ray import train

def trainer(config):
    for epoch in range(config["epochs"]):
        loss = compute_loss(epoch)
        train.report({"val_loss": loss, "epoch": epoch})              # → Ray dashboard
        print(json.dumps({                                            # → DING
            "metric": "val_loss",
            "value": loss,
            "epoch": str(epoch),  # cast to string so the template variable resolves
        }))

if __name__ == "__main__":
    ray.init()
    # ... orchestrate ray.train / ray.tune workload ...
```

Submit:

```bash
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
ray start --head
ray job submit --address=http://localhost:8265 \
  --runtime-env-json='{"env_vars": {"SLACK_WEBHOOK_URL": "'"$SLACK_WEBHOOK_URL"'"}}' -- \
  ding run --config ding.yaml -- python train.py
```

The `--runtime-env-json` block forwards the Slack webhook to the driver process Ray spawns; without it, the driver's environment won't have access to the variable.

### Path B: bare script (manual labels)

For users who already have `ray.init()` inside a regular script and don't want to switch to `ray job submit`. `RAY_JOB_ID` is **not** set in this path; runner falls back to `local`. Emit `ray_job_id` (or whatever scope you need) as a JSON event label so alerts still carry the right run identifier:

```python
import json, ray, uuid

ray_job_id = str(uuid.uuid4())  # or pull from your own job-id source
ray.init()

for epoch in range(epochs):
    loss = train_epoch()
    print(json.dumps({
        "metric": "val_loss",
        "value": loss,
        "epoch": str(epoch),
        "ray_job_id": ray_job_id,
        "runner": "ray",
    }))
```

DING's JSON ingester extracts flat top-level string keys as event labels and flat number keys as floats; nested objects are skipped. Keep `ray_job_id` and `runner` as flat top-level keys (not nested under a `labels:` object).

Run with `ding run --config ding.yaml -- python train.py`. Alerts will be tagged with the user-supplied `ray_job_id` label rather than runctx-derived `run_id`.

## What you get

A Slack message during training when `val_loss` exceeds threshold:

> 🔔 `loss_spike`
> val_loss spike: 12.4 on epoch 7 (Ray job raysubmit_abcdef1234567890)

…and on training-process exit:

> 🔔 `training_failed`
> Ray job raysubmit_abcdef1234567890 failed (exit 1)

All Path A alerts are auto-tagged with `run_id` + `runner=ray`. The `run_id` matches the UUID printed by `ray job list`.

## Configuration

`runctx` auto-detects Ray when `RAY_JOB_ID` is set in the entry point's environment (always set by `ray job submit`):

| Label | Source env var | Notes |
|---|---|---|
| `run_id` | `RAY_JOB_ID` | Ray job UUID matching `ray job list` output |
| `runner` | `"ray"` (set by runctx) | |

Use these in `match.labels` or `message` template variables. See [Configuration](../configuration.md) for the full reference.

## Verification

```bash
pip install "ray[default]"
mkdir ray-smoke && cd ray-smoke
# Author ding.yaml + train.py per the example above; have train.py exit non-zero.
ray start --head
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
ray job submit --address=http://localhost:8265 \
  --runtime-env-json='{"env_vars": {"SLACK_WEBHOOK_URL": "'"$SLACK_WEBHOOK_URL"'"}}' -- \
  ding run --config ding.yaml -- python train.py
# Verify in Slack:
#   1. training_failed message fires within ~5s of script exit
#   2. run_id label matches `ray job list` UUID
#   3. runner label == "ray"
ray job list
ray stop
```

If the alert doesn't fire, check the Ray driver logs (`ray job logs <id>`) for `ding` output. Common issues: `SLACK_WEBHOOK_URL` not forwarded via `--runtime-env-json`, or `drain_timeout` shorter than the notifier retry window — see [Configuration](../configuration.md).

## Tradeoffs / known limitations

- **Trial-level Tune labels not auto-detected.** `ray.tune` trial IDs (`tune.get_context().get_trial_id()`) live in Python-only context APIs, not env vars. For trial-scoped labels, emit them as JSON event labels from your training function — runctx labels are job-scoped, not trial-scoped. A Tier-2 helper (`pip install ding-ray`) is flagged for this; not built yet.
- **`ray.init()` bare scripts don't set `RAY_JOB_ID`.** The env var is set only by `ray job submit`. A bare `python train.py` with `ray.init()` falls back to `runner=local`. Use Path B above if you need run-scoped labels in this case, or switch to `ray job submit`.
- **Driver vs. worker.** `ding run` wraps the Ray driver. Worker tasks (`@ray.remote` functions) emit metrics via Ray's stdout aggregation, which propagates back to the driver's stdout — DING sees them in driver-side line capture without worker-side instrumentation.
- **Autoscaler-killed nodes look identical to normal exits.** When Ray's autoscaler reaps an idle worker, DING sees no event — the wrapped driver process keeps running. Use the Ray dashboard for fleet-health monitoring; DING handles alerting on the driver's metrics and exit.

> **KubeRay / `RayJob` CRD**: works the same way — `RAY_JOB_ID` is set when the job submission triggers the entry point container in the Ray head pod. No DING-side recipe variant needed; reuse the Path A pattern with `ding run` as the RayJob's entry point command.

## Escalation criteria

This recipe is **Tier 1** by the program's standard rubric, with a Tier-2 promotion candidate flagged:

- **Setup commands required:** 2 (`pip install`, `ray start --head`) — under threshold of 5
- **Boilerplate lines:** ~40 — under threshold of 50
- **"Gotcha" callouts:** 4 — over threshold of 2 → **promotion trigger**
- **End-to-end runnable:** yes (Ray is OSS; runs locally in single-node mode)

A future Tier-2 candidate worth tracking: **`pip install ding-ray` Python helper** — wraps `tune.get_context().get_trial_id()` / `train.get_context().get_trial_id()` and emits trial-scoped JSON labels automatically alongside DING events. Separate repo (parallel to `ding-action`), pip-installable. Not built here.
