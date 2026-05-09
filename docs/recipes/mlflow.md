# Using DING with MLflow

> MLflow is the leading open-source platform for managing the ML lifecycle — experiment tracking, model registry, project orchestration. DING's `ding run` wraps an MLproject entry point, evaluates rules during the training run, and fires alerts on metric thresholds and exit code — both during the run *and* on exit, with the alert linking back to the MLflow UI.

## Prerequisites

- DING binary `>= v0.10.0` — see [install](../install.md)
- `mlflow >= 2.0` (`pip install mlflow`)
- An MLflow tracking URI: local SQLite for dev; remote tracking server like Databricks or self-hosted (`mlflow server`) for production deep-links to work
- A notifier endpoint (Slack webhook URL is the canonical example)

## Minimal example

The shortest config that wires DING into an MLflow project so alerts fire during training and on exit.

`MLproject` (project root):

```yaml
name: my-training
entry_points:
  main:
    parameters:
      epochs: { type: int, default: 10 }
    command: "ding run --config ding.yaml -- python train.py --epochs {epochs}"
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
    message: "val_loss spike: {{ .value }} on epoch {{ .epoch }} (run {{ .run_id }})"
    alert:
      - notifier: slack

  # On exit: fire if the training process exits non-zero.
  # The synthetic run.exit event is dispatched at end-of-run; a default
  # (during-run) rule matching it fires once when the wrapped command exits.
  - name: training_failed
    match: { metric: run.exit }
    condition: value > 0
    message: |
      MLflow run failed (exit {{ .exit_code }} after {{ .duration_seconds }}s)
      <{{ .tracking_uri }}/#/experiments/{{ .experiment_id }}/runs/{{ .run_id }}|View run in MLflow UI>
    alert:
      - notifier: slack
```

`train.py` (excerpt — emit JSON events for DING alongside MLflow's native logging):

```python
import json, mlflow

with mlflow.start_run():
    for epoch in range(epochs):
        loss = train_epoch()
        mlflow.log_metric("val_loss", loss, step=epoch)              # → MLflow tracking server
        print(json.dumps({                                           # → DING
            "metric": "val_loss",
            "value": loss,
            "epoch": str(epoch),  # cast to string so the template variable resolves
        }))
```

Invoke with:

```bash
mlflow run . --env-manager=local -P epochs=20
```

## What you get

A Slack message during training when `val_loss` exceeds threshold:

> 🔔 `loss_spike`
> val_loss spike: 12.4 on epoch 7 (run abc123def456)

…and on training-process exit:

> 🔔 `training_failed`
> MLflow run failed (exit 1 after 42s)
> [View run in MLflow UI](#)

The deep-link in the second message takes you straight to the MLflow run page. All alerts are auto-tagged with `run_id`, `runner=mlflow`, `experiment_id`, `tracking_uri`.

## Configuration

`runctx` auto-detects MLflow when `MLFLOW_RUN_ID` is set in the entry point's environment (always set by `mlflow run`):

| Label | Source env var | Notes |
|---|---|---|
| `run_id` | `MLFLOW_RUN_ID` | the MLflow run UUID |
| `runner` | `"mlflow"` (set by runctx) | |
| `experiment_id` | `MLFLOW_EXPERIMENT_ID` | enables Slack-channel routing per experiment |
| `tracking_uri` | `MLFLOW_TRACKING_URI` | only set when value starts with `http://` or `https://`; local file paths skipped |

Use these in `match.labels` or `message` template variables. See [Configuration](../configuration.md) for the full notifier reference.

## Verification

```bash
pip install mlflow
mkdir mlflow-smoke && cd mlflow-smoke
# Author MLproject, train.py (with intentional non-zero exit), ding.yaml per the example above
mlflow server --host 127.0.0.1 --port 5000 &
export MLFLOW_TRACKING_URI=http://127.0.0.1:5000
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
mlflow run . --env-manager=local
# Verify in Slack:
#   1. training_failed message fires within ~5s of the script exit
#   2. tracking URI deep-link is clickable and lands on the MLflow run page
#   3. labels include run_id, experiment_id, tracking_uri
```

If the alert doesn't fire, check the `mlflow run` log for `ding` output. Common issues: `SLACK_WEBHOOK_URL` not exported in the shell that ran `mlflow run`, or `drain_timeout` shorter than the notifier retry window — see [Configuration → drain_timeout](../configuration.md#drain_timeout-and-retry-behaviour-in-ding-run).

## Tradeoffs / known limitations

- **Bare scripts not auto-detected.** Auto-detection requires the MLproject pattern (DING as entry point command). Running `python train.py` directly produces `runner=local`; emit `mlflow_run_id` as a JSON event label inside the script if you need it on alerts.
- **Local-file tracking URIs don't deep-link.** When `MLFLOW_TRACKING_URI=./mlruns` (the MLflow default), `tracking_uri` is omitted by runctx and Slack templates referencing it render an empty link. Use a real tracking server for deep-links.
- **`mlflow run` env-manager defaults to conda.** This recipe uses `--env-manager=local` to use the host environment where DING is on PATH. For isolation, install DING into the conda env via `conda.yaml` or use an absolute path in the MLproject command.
- **DING is alerting; MLflow is tracking.** They coexist. Emit metrics to both: `mlflow.log_metric` for MLflow's UI, `print(json.dumps(...))` for DING rules. DING fires real-time alerts; MLflow records history. Different purposes; no overlap.

## Escalation criteria

This recipe is **Tier 1** by the program's standard rubric:

- **Setup commands required:** 1 (`pip install mlflow`) — under threshold of 5
- **Boilerplate lines:** ~30 (3 files combined) — under threshold of 50
- **"Gotcha" callouts:** 4 — over threshold of 2
- **End-to-end runnable:** yes (MLflow is OSS; `mlflow server` is self-hostable)

The 4 gotchas are conceptual ("DING ≠ MLflow tracking; bare scripts not auto-detected; conda env defaults; deep-links require remote tracking server"), not boilerplate-driven — a Tier-2 abstraction wouldn't reduce them.

A future Tier-2 candidate worth tracking: **`type: mlflow_run_tag` notifier** — writes the alert as a tag on the active MLflow run, surfacing failure context in the MLflow UI alongside metrics. Not built here.
