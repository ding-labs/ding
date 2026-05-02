# DING

> **Alerting that ships with the workload.**
> One binary. Drops into your CI job, your ML training run, your batch pipeline.
> Don't store it. Stream it. DING it.

```
$ brew install ding-labs/tap/ding
```

```
$ curl -sf https://start.ding.ing | sh
```

[Docker, binary](#install) · [ding.ing](https://ding.ing)

---

## What this is

DING runs **with** your workload, not next to it. The job emits events; DING evaluates rules in-process; alerts fire during the run *and* a summary fires when the job exits. Both die together. No agents. No dashboards. No cloud account.

Most observability tools are shaped for long-running fleets — pull metrics from steady-state services into a central database, alert on the database. That shape doesn't fit ephemeral compute (a 4-minute CI job, a 90-minute training run, a 30-second batch ETL, a 10-minute game match). DING is shaped for ephemeral compute.

```
                      ┌─ DING fires alerts during the run
                      │
   ┌─── your job ─────┼─────────── exits ─┐
   │                  │                   │
   │  emits JSON      │                   │  end-of-run rules
   │  events to       │                   │  fire here, with
   │  stdout          │                   │  aggregate stats
   └──────────────────┴───────────────────┘
                      │
                      └─ alerts include run_id, branch,
                         commit, exit code, duration
```

---

## 60-second example: alert on a flaky test suite

`.github/workflows/ci.yml`:

```yaml
- run: |
    curl -sf https://start.ding.ing | sh
    ding run --config alerts.yaml -- pytest tests/
```

`alerts.yaml`:

```yaml
rules:
  # Fires immediately on any test that takes longer than 5 seconds.
  - name: slow_test
    match: { metric: test.duration }
    condition: value > 5
    message: "slow test {{ .test }} on {{ .branch }}: {{ .value }}s"
    alert: [{ notifier: github_actions }]

  # Fires once at end of run if the job's average test latency was elevated.
  - name: regression
    match: { metric: test.duration }
    mode: end-of-run
    condition: avg(value) over 1h > 1
    message: "p50 test latency was {{ .avg }}s (count={{ .count }})"
    alert: [{ notifier: github_actions }]

  # Fires if pytest exits non-zero.
  - name: failed
    match: { metric: run.exit }
    condition: value > 0
    message: "pytest failed with exit code {{ .value }}"
    alert: [{ notifier: github_actions }]
```

In your test, emit JSON to stdout however you like:

```python
print(json.dumps({"metric": "test.duration", "value": elapsed, "test": name}))
```

Three things happen:
- During the run, `slow_test` alerts surface as **GitHub Actions warnings** in the PR check.
- When `pytest` exits, end-of-run summary appears in **the workflow's step summary** with markdown formatting.
- DING exits with `pytest`'s exit code, so the check stays red on test failure.

Run-context labels (`run_id`, `branch`, `commit`, `repo`, `workflow`) auto-attach to every alert. Nothing to configure.

---

## How it works

### `ding run` wraps your command

```
ding run [flags] -- <command> [args...]
```

DING starts your command, mirrors its stdout/stderr to yours, parses JSON-line (or Prometheus-text) events from the output, and evaluates rules against them in real time. Non-event lines pass through unchanged.

When your command exits, DING:

1. Emits a synthetic `run.exit` event with the exit code and run duration.
2. Fires any `mode: end-of-run` rules with the accumulated state.
3. Exits with your command's exit code.

SIGTERM and SIGINT are forwarded to the child for graceful shutdown.

### Run context, auto-detected

DING reads the runner's environment variables and attaches labels automatically. No config required.

| Runner | Detected via | Auto-attached labels |
|---|---|---|
| GitHub Actions | `GITHUB_ACTIONS=true` | `run_id`, `runner`, `repo`, `branch`, `commit`, `workflow`, `job`, `actor`, `event` |
| GitLab CI | `GITLAB_CI=true` | `run_id`, `runner`, `repo`, `branch`, `commit`, `job` |
| CircleCI | `CIRCLECI=true` | `run_id`, `runner`, `repo`, `branch`, `commit`, `job` |
| Jenkins | `JENKINS_URL` set | `run_id`, `runner`, `job`, `build` |
| Buildkite | `BUILDKITE=true` | `run_id`, `runner`, `repo`, `branch`, `commit` |
| Argo Workflows | `ARGO_TEMPLATE` set | `run_id`, `runner`, `workflow`, `node`, `pod`, `namespace` |
| MLflow | `MLFLOW_RUN_ID` set | `run_id`, `runner`, `experiment_id`, `tracking_uri` |
| (anything else) | — | `run_id` (random hex), `runner=local` |

User-supplied event labels always win over auto-detected ones — DING never clobbers your labels.

### Two rule modes

```yaml
rules:
  # Default: fires whenever the condition is true (event-by-event or windowed).
  - name: spike
    condition: value > 95
    cooldown: 1m
    # mode: during-run    ← default, can be omitted

  # Fires once at end of run, evaluated against accumulated state.
  - name: summary
    condition: avg(value) over 1h > 50
    mode: end-of-run
    # No cooldown — end-of-run rules fire at most once per run.
```

`during-run` and `end-of-run` rules coexist freely. The same `latency` metric can drive a real-time spike alert *and* an end-of-run regression summary.

### The `run.exit` synthetic event

When the wrapped command exits, DING emits an event with:

- `metric: run.exit`
- `value: <exit code>` (also in `Floats.exit_code`)
- `Floats.duration_seconds: <seconds since start>`
- All run-context labels

Match it like any other metric:

```yaml
- name: nonzero_exit
  match: { metric: run.exit }
  condition: value > 0
  message: "job failed with exit code {{ .value }} after {{ .duration_seconds }}s"
  alert: [{ notifier: github_actions }]
```

---

## Rules

One YAML file. Lives in your repo. Ships with your code.

```yaml
rules:
  - name: cpu_spike
    match: { metric: cpu_usage }
    condition: value > 95
    cooldown: 1m
    message: "CPU spike on {{ .host }}: {{ .value }}%"
    alert: [{ notifier: stdout }]

  - name: cpu_sustained
    match: { metric: cpu_usage }
    condition: avg(value) over 5m > 80
    cooldown: 10m
    message: "Sustained high CPU: {{ .avg }}% avg on {{ .host }}"
    alert: [{ notifier: stdout }]
```

Condition forms:

```
value > 95                       # single event
avg(value) over 5m > 80          # average over window
max(value) over 1m >= 100
min(value) over 10s < 10
sum(value) over 30s > 0
count(value) over 2m > 50        # number of events, not sum
```

Compound conditions with `AND` / `OR` are supported.

Template variables in `message:`:

| Variable | When | Description |
|---|---|---|
| `.metric` | always | metric name |
| `.value` | always | raw event value |
| `.rule` | always | rule name |
| `.fired_at` | always | RFC3339 timestamp |
| `.run_id`, `.branch`, `.commit`, … | run mode | run-context labels |
| `.host`, `.region`, … | always | any user label |
| `.avg` `.max` `.min` `.sum` `.count` | windowed only | aggregate result |

---

## Notifiers

Three are built in: `stdout`, `github_actions`, plus user-defined `webhook` notifiers.

### `github_actions` — CI-native output

Writes alerts as GitHub Actions inline annotations (`::warning::`) so they appear in the live log and the PR check, *and* renders a markdown section in `$GITHUB_STEP_SUMMARY` for the workflow run page.

```yaml
rules:
  - name: slow
    condition: value > 5
    alert: [{ notifier: github_actions }]
```

Outside Actions, falls back to plain stdout — safe to use everywhere.

### `webhook`

```yaml
notifiers:
  alert-slack:
    type: webhook
    url: https://hooks.slack.com/services/T.../B.../...
    max_attempts: 3       # retries on 5xx (default: 3)
    initial_backoff: 1s   # doubles each attempt (default: 1s)

rules:
  - name: cpu_spike
    condition: value > 95
    cooldown: 1m
    alert:
      - notifier: stdout
      - notifier: alert-slack
```

The webhook receives a JSON POST:

```json
{"rule":"cpu_spike","message":"CPU spike on web-01: 97%",
 "metric":"cpu_usage","value":97.0,"fired_at":"...",
 "host":"web-01","run_id":"...","branch":"main"}
```

4xx responses are dropped. 5xx responses are retried with exponential backoff.

---

## Recipes

Looking for a config that works on your specific platform? See **[docs/recipes/](docs/recipes/index.md)** for platform-specific guides:

- **CI/CD:** [GitHub Actions](https://github.com/ding-labs/ding-action) · [GitLab CI](docs/recipes/gitlab-ci.md) · [CircleCI](docs/recipes/circleci.md) · [Jenkins](docs/recipes/jenkins.md) · [Buildkite](docs/recipes/buildkite.md)
- **Orchestration:** [Kubernetes Jobs / CronJobs](docs/recipes/kubernetes-jobs.md) · [Argo Workflows](docs/recipes/argo-workflows.md)
- **ML:** [MLflow](docs/recipes/mlflow.md)
- More platforms (Ray, dbt, Modal, …) coming in subsequent waves.

---

## Beyond CI — long-running mode

`ding run` is the new wedge. The original mode still exists:

```
ding serve --config ding.yaml
```

This runs DING as a long-lived HTTP server on `:8080` accepting `POST /ingest`, `GET /health`, `GET /rules`, `POST /reload`, `GET /metrics`. Use it for:

- Persistent services (`your-app | ding serve`)
- Fleet-wide alerting from many short-lived clients
- Hot-reloading rules via SIGHUP or `POST /reload`

Persist state across restarts:

```yaml
persistence:
  state_file: /var/lib/ding/state.json
  flush_interval: 30s
```

SIGTERM / SIGINT — drains in-flight requests, flushes state, exits 0.

---

## Why

> **Fires alerts in 4ms.** Prometheus default scrape + eval + Alertmanager dispatch: ~62 seconds minimum. That's not a knock on Prometheus — it's a pull-based system built for persistence and fleet-wide aggregation. DING is push-based and stateless. The architecture is the difference.

The architecture choices that make `ding run` possible are the same ones that always made DING fast:

- **Stateless** — nothing to provision, nothing to clean up when the job dies
- **5MB static binary, 9ms cold start** — small enough to ship inside a CI job, fast enough that it doesn't add latency to your pipeline
- **Push-based** — events flow at the speed of your job, no scrape interval to tune
- **Windowed aggregations in memory** — `avg(value) over 5m` works without a database
- **Per-labelset cooldowns** — `web-01` being loud doesn't silence `web-02`; one flaky test doesn't silence another
- **Config in your repo** — alerting is a dev artifact, ships with the code that emits the events
- **Composable** — stdin in, JSON lines out, pipes into anything

---

## Performance

| Metric | Result | Context |
|---|---|---|
| Alert latency p50 | **4ms** | p99: 16ms — Prometheus default: ~62s |
| Requests / second | **116k** | 50 concurrent workers, 30s window |
| Cold start p50 | **9ms** | fork → first /health — Prometheus: 185ms |
| Per rule evaluation | **106ns** | simple threshold — windowed: 157ns |

Benchmarked 2026-03-23 on Apple M3. [Full methodology and raw results →](https://github.com/ding-labs/ding/blob/main/BENCHMARKS.md)

---

## Input formats

JSON lines:

```json
{"metric": "cpu_usage", "value": 92.5, "host": "web-01"}
```

Prometheus text:

```
cpu_usage{host="web-01"} 92.5
```

Either is accepted from `ding run` subprocess output, `ding serve` HTTP/stdin, or piped stdin. Auto-detected by default; force a format with `server.format: json` or `prometheus`.

---

## CLI

```
ding run -- <cmd> [args...]      Wrap a command; alert on its events
ding serve                       Run as an HTTP alerting daemon
ding validate                    Check ding.yaml for errors
ding version                     Print version
```

Each command takes `--config <path>` (default `ding.yaml`).

---

## Install

**Homebrew:**

```
brew install ding-labs/tap/ding
```

**Binary:**

```
curl -sf https://start.ding.ing | sh
```

**Docker:**

```
docker run -v ./ding.yaml:/etc/ding/ding.yaml \
  ghcr.io/ding-labs/ding
```

**GitHub Actions:** see [ding-labs/ding-action](https://github.com/ding-labs/ding-action) — one `uses:` line.

---

[Apache-2.0](./LICENSE) · [ding.ing](https://ding.ing)
