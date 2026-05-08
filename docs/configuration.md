# Configuration

DING is configured via a single YAML file (default: `ding.yaml`). All sections are optional except `rules`.

```bash
ding validate --config ding.yaml   # check before deploying
ding serve --config ding.yaml
```

---

## Full example

```yaml
server:
  port: 8080
  format: auto
  jq: '.events[] | {metric: .name, value: .reading, host: .tags.host}'
  max_buffer_size: 10000
  read_timeout: 5s
  write_timeout: 10s
  idle_timeout: 60s
  max_body_bytes: 1048576

notifiers:
  slack:
    type: slack
    url: https://hooks.slack.com/services/T.../B.../...
    max_attempts: 3
    initial_backoff: 1s

rules:
  - name: cpu_spike
    match:
      metric: cpu_usage
      region: us-east
    condition: value > 95
    cooldown: 1m
    message: "CPU spike on {{ .host }}: {{ .value }}%"
    alert:
      - notifier: slack

  - name: cpu_sustained
    match:
      metric: cpu_usage
    condition: avg(value) over 5m > 80
    cooldown: 10m
    message: "Sustained high CPU: {{ .avg }}% avg on {{ .host }}"
    alert:
      - notifier: stdout

persistence:
  state_file: /var/lib/ding/state.json
  flush_interval: 30s

alert_log:
  path: /var/log/ding/alerts.jsonl
```

---

## `server`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `8080` | HTTP listen port |
| `format` | string | `auto` | Input format: `json`, `prometheus`, or `auto` (auto-detects per request) |
| `jq` | string | — | Optional [jq](https://jqlang.github.io/jq/) filter applied to every inbound payload before rule evaluation. Output must produce objects with `metric` and `value` fields. |
| `max_buffer_size` | int | `10000` | Maximum events retained per rule per label-set for windowed aggregations |
| `read_timeout` | duration | `5s` | HTTP read timeout |
| `write_timeout` | duration | `10s` | HTTP write timeout |
| `idle_timeout` | duration | `60s` | HTTP idle connection timeout |
| `max_body_bytes` | int64 | `1048576` | Maximum request body size in bytes (1MB). Returns 413 on overflow. |
| `drain_timeout` | duration | `5s` | How long `ding run` waits for notifier delivery queues to flush on exit before force-stopping. See note below. |

#### `drain_timeout` and retry behaviour in `ding run`

`ding run` exits as soon as the wrapped command finishes, so notifier delivery must complete within the drain window. The default `5s` covers a single fast delivery comfortably, but retry attempts eat into that window. With the default `initial_backoff: 1s` and `max_attempts: 3`, a full retry cycle takes at least `1 + 2 + 4 = 7s` — longer than the default drain timeout.

If your notifier is flaky and you want retries to have a real chance:

```yaml
server:
  drain_timeout: 10s   # must exceed initial_backoff * 2^max_attempts

notifiers:
  slack:
    type: slack
    url: https://hooks.slack.com/...
    max_attempts: 3
    initial_backoff: 1s   # retry window: 1 + 2 = 3s (fits in 10s)
```

If fast CI exit matters more than retry guarantees, keep `drain_timeout` short and set `max_attempts: 1`.

---

## `notifiers`

A map of named notifiers. Reference them by name in rule `alert` blocks.

**Built-in notifiers** — always available without declaration:

| Name | Description |
|------|-------------|
| `stdout` | Writes every alert as a JSON line to stdout |
| `github_actions` | Emits `::warning::` annotations and appends a markdown summary to `$GITHUB_STEP_SUMMARY`. Falls back gracefully outside Actions. |

**Configured notifiers:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | — | `slack`, `discord`, `webhook`, or `github_actions` |
| `url` | string | — | Destination URL (required for `slack` and `webhook`) |
| `max_attempts` | int | `3` | Total delivery attempts including the first (slack/webhook only) |
| `initial_backoff` | duration | `1s` | First retry delay; doubles each attempt (slack/webhook only) |

### `type: slack`

Posts a [Block Kit](https://api.slack.com/block-kit) message to a Slack incoming webhook URL. Run-context fields are surfaced automatically as structured fields when present — no template work required.

When used with `ding run`, the following fields appear in the Slack message if DING detected them from the CI environment:

| Field | Source | Example |
|-------|--------|---------|
| exit code | `run.exit` float | `1` |
| duration | `run.exit` float | `42.5s` |
| branch | CI env auto-detect | `main` |
| commit | CI env auto-detect | `abc1234` (truncated) |
| repo | CI env auto-detect | `acme/api` |
| workflow | CI env auto-detect | `CI` |
| job | CI env auto-detect | `test` |
| actor | CI env auto-detect | `octocat` |
| runner | CI env auto-detect | `github-actions` |
| run id | CI env auto-detect | `12345` |

Up to 10 fields are shown. Exit code and duration are prioritized — they always appear when present, even if many label fields would otherwise fill the limit.

### `type: discord`

Posts a Discord [embed](https://discord.com/developers/docs/resources/message#embed-object) to an incoming webhook URL. Run-context fields are surfaced automatically as embed fields when present — no template work required.

When used with `ding run`, the following fields appear in the Discord embed if DING detected them from the CI environment:

| Field | Source | Example |
|-------|--------|---------|
| exit code | `run.exit` float | `1` |
| duration | `run.exit` float | `42.5s` |
| branch | CI env auto-detect | `main` |
| commit | CI env auto-detect | `abc1234` (truncated) |
| repo | CI env auto-detect | `acme/api` |
| workflow | CI env auto-detect | `CI` |
| job | CI env auto-detect | `test` |
| actor | CI env auto-detect | `octocat` |
| runner | CI env auto-detect | `github-actions` |
| run id | CI env auto-detect | `12345` |

All fields are rendered inline. Discord allows up to 25 fields per embed; exit code and duration are prioritized and always appear when present.

### `type: kubernetes_event`

Publishes alerts as native Kubernetes [Events](https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/event-v1/) (`corev1.Event`), visible to `kubectl describe pod` and `kubectl get events`. Available only when DING is running inside a Kubernetes Pod (in-cluster ServiceAccount auth — `kubeconfig` files are not supported).

```yaml
notifiers:
  k8s:
    type: kubernetes_event
    namespace: ""              # default: POD_NAMESPACE downward API
    event_reason: DingAlertFired   # default
    event_type: Warning            # "Normal" or "Warning"; default Warning
    max_attempts: 3
    initial_backoff: 1s
```

| Field | Default | Notes |
|-------|---------|-------|
| `namespace` | `POD_NAMESPACE` env (downward API) | override target namespace if needed |
| `event_reason` | `DingAlertFired` | K8s convention is short PascalCase |
| `event_type` | `Warning` | only `Normal` and `Warning` accepted |
| `max_attempts` | `3` | inherited default |
| `initial_backoff` | `1s` | inherited default |

**Required Pod env (downward API)**: `POD_NAME`, `POD_UID`, `POD_NAMESPACE`, `NODE_NAME`. The K8s recipe at [docs/recipes/kubernetes-jobs.md](recipes/kubernetes-jobs.md) shows the canonical manifest fragment that surfaces these. The Event's `involvedObject` is the Pod where DING is running (cheap, no API lookup).

**Required RBAC**: `events.create` in the Pod's namespace. Minimal Role:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: { name: ding-event-publisher }
rules:
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create"]
```

Bind to the Pod's ServiceAccount via a RoleBinding. K8s aggregates duplicate Events (same `involvedObject` + `reason` + `message` within a window) into one Event with `count` incremented; DING's per-rule `cooldown` still applies on top. Forbidden (RBAC denied), Unauthorized, BadRequest, and Invalid responses are permanent (logged + dropped without retry); 5xx and network errors retry up to `max_attempts`.

### `type: gitlab_artifact`

Writes alert Markdown to a file the user declares in `.gitlab-ci.yml` `artifacts:` so DING alerts surface as a downloadable pipeline artifact in the GitLab job UI. No external service required.

```yaml
notifiers:
  artifact:
    type: gitlab_artifact
    path: ding-alerts.md   # default; relative to current working directory
```

| Field | Default | Notes |
|-------|---------|-------|
| `path` | `ding-alerts.md` | Relative path resolved against the process's CWD (= `$CI_PROJECT_DIR` in GitLab CI). Absolute paths also work. |

**Behavior**: sync, mutex-guarded, append-only. The first `Send()` writes a `# DING Alerts` H1 header; subsequent calls append `## <rule>` sections with metric, value, fired_at, optional aggregates, and sorted-key label list. No async queue, no retry, no metrics — failures (permission denied, disk full) are returned from `Send()` and logged.

**No CI gate**: the notifier writes the file regardless of whether it's running in GitLab CI. Outside CI, it just produces a local `ding-alerts.md` — harmless. Combine with `.gitlab-ci.yml` `artifacts: { when: always, paths: [ding-alerts.md] }` to archive the file on every pipeline run (including failed jobs). See the [GitLab CI recipe](recipes/gitlab-ci.md#native-gitlab-ui-surfacing) for an end-to-end example.

### `type: buildkite_annotate`

Publishes alerts as Buildkite build annotations via `buildkite-agent annotate`. All alerts for a build land in a single rolling annotation (`--context ding --append`) shown at the top of the Buildkite job UI. Requires `buildkite-agent` on PATH (always set inside Buildkite jobs); outside Buildkite the notifier no-ops gracefully after a one-time warning.

```yaml
notifiers:
  annotate:
    type: buildkite_annotate
    style: error    # success | info | warning | error; default error
```

| Field | Default | Notes |
|-------|---------|-------|
| `style` | `error` | Buildkite annotation style. Drives the colored badge in the build UI. |

**Behavior**: sync, mutex-guarded. The first `Send()` writes a `# DING Alerts` H1 header; subsequent calls append `## <rule>` sections that Buildkite's `--append` concatenates into the existing annotation body. No async queue, no retry, no metrics — failures from `buildkite-agent` (agent disconnected, body too large, etc.) are returned from `Send()` with stderr captured.

**No CI gate**: the notifier checks for `buildkite-agent` once at construction; outside Buildkite jobs it logs `ding: buildkite_annotate notifier: buildkite-agent not on PATH; alerts via this notifier will be no-ops` and Send becomes a no-op. See the [Buildkite recipe](recipes/buildkite.md#native-buildkite-ui-surfacing) for an end-to-end example.

### `type: webhook`

Posts a flat JSON payload to any HTTP endpoint. Useful for generic integrations (PagerDuty, custom receivers, etc.).

Payload shape:

```json
{
  "rule": "cpu_spike",
  "message": "CPU spike on web-01: 97%",
  "metric": "cpu_usage",
  "value": 97.0,
  "fired_at": "2026-04-25T10:00:00Z",
  "host": "web-01"
}
```

All event labels (including run-context labels when using `ding run`) are merged into the top-level payload object. 4xx responses are dropped. 5xx responses are retried with exponential backoff.

---

## `rules`

A list of alerting rules. Rules are evaluated independently; each has its own cooldown and buffer state per label-set.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Rule identifier, used in alert payloads |
| `match` | map | no | Label filters. Only events matching all key-value pairs are evaluated by this rule. Omit to match all events. |
| `match.metric` | string | no | Metric name filter |
| `condition` | string | yes | Evaluation expression (see below) |
| `cooldown` | duration | no | Minimum time between consecutive alerts for the same label-set |
| `message` | string | no | Alert message template (Go `text/template` syntax) |
| `alert` | list | yes | List of `{notifier: <name>}` targets |

### Condition syntax

**Single-event (threshold):**

```
value > 95
value >= 80
value < 10
value <= 5
value == 0
value != 42
```

**Windowed aggregation:**

```
avg(value) over 5m > 80
max(value) over 1m >= 100
min(value) over 10s < 5
sum(value) over 30s > 1000
count(value) over 2m > 50
```

**Compound (AND / OR):**

```
value > 90 AND avg(value) over 5m > 80
value < 5 OR count(value) over 1m > 100
```

Comparison operators: `>`, `>=`, `<`, `<=`, `==`, `!=`

### Message template variables

| Variable | Available | Description |
|----------|-----------|-------------|
| `.metric` | always | Metric name |
| `.value` | always | Raw event value |
| `.rule` | always | Rule name |
| `.fired_at` | always | RFC3339 timestamp |
| `.<label>` | always | Any label from the event (e.g., `.host`, `.region`) |
| `.avg` | windowed | Average over window |
| `.max` | windowed | Maximum over window |
| `.min` | windowed | Minimum over window |
| `.sum` | windowed | Sum over window |
| `.count` | windowed | Event count over window |

### Per-label-set cooldowns

Cooldowns are tracked independently per unique label combination. A noisy `web-01` does not suppress alerts from `web-02`.

---

## `persistence`

Optional. Persists cooldown state and windowed ring buffers to disk so DING survives restarts without losing alert history.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `state_file` | string | — | Path to JSON snapshot file |
| `flush_interval` | duration | `30s` | How often to write the snapshot while running |

On startup, DING restores from the snapshot file if it exists. On reload (`SIGHUP` or `POST /reload`), state is flushed before the new config is loaded.

---

## `alert_log`

Optional. Appends every fired alert as a JSON line to a file.

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | Path to the log file. Created if it does not exist. |

Each line is a JSON object matching the webhook payload format.

---

## Duration format

All duration fields accept Go duration strings: `5s`, `1m`, `2h`, `500ms`.

## Environment variable substitution

DING expands `${VAR}` references in `ding.yaml` against the process environment when the file is loaded. This lets you keep secrets (Slack URLs, PagerDuty routing keys, API tokens) out of version control.

### Syntax

Reference an environment variable as `${VAR}`. Variable names match `[A-Za-z_][A-Za-z0-9_]*`.

### Behavior at a glance

| In `ding.yaml` | Environment | Result |
|---|---|---|
| `url: ${SLACK_URL}` | `SLACK_URL=https://hooks...` | `url: https://hooks...` |
| `url: https://${HOST}/api` | `HOST=example.com` | `url: https://example.com/api` |
| `token: ${A}-${B}` | `A=abc`, `B=xyz` | `token: abc-xyz` |
| `path: /tmp/${X}/${X}` | `X=foo` | `path: /tmp/foo/foo` (repeats fine) |
| `note: ${A}` | `A=""` | `note: ""` (empty value is allowed) |
| `url: ${MISSING}` | `MISSING` not set | **load fails:** `unset env vars referenced in config: MISSING` |
| `url: ${A}; token: ${B}` | neither set | **load fails:** `unset env vars referenced in config: A, B` (both reported, sorted) |
| `name: $SHELL_STYLE` | `SHELL_STYLE=x` | `name: $SHELL_STYLE` (no expansion — braces are required) |
| `name: ${WITH-DASH}` | any | `name: ${WITH-DASH}` (`-` not allowed in variable names — passes through) |
| `name: ${}` | any | `name: ${}` (empty braces — passes through) |

### Footgun: YAML metacharacters

Substitution is a raw-text replace performed *before* YAML parsing. If a variable's value might contain newlines, colons, or quotes, wrap the field in quotes:

```yaml
url: "${MIGHT_CONTAIN_SPECIAL}"
```

For typical secrets (Slack URLs, PagerDuty tokens, API keys, opaque ID strings) this is never an issue.

### Out of scope

- Bare `$VAR` (no braces) is not expanded.
- No `${VAR:-default}` for inline defaults — set the env var to the default before launching DING.
- No `$${VAR}` escape for writing literal `${VAR}` — the use case is rare; if you hit it, file an issue.

## Testing rules without a workload

DING ships two preview surfaces so you can verify rules before turning on real notifications.

### `ding test-rule` — replay synthetic events

Pipe or pass JSONL events at a config; matching rules render messages as if they were about to fire, but no notifications go out.

```sh
# Pipe events from any source
echo '{"metric":"loss","value":1.5}' | ding test-rule --config ding.yaml

# Read from a file (use - for explicit stdin)
ding test-rule events.jsonl
```

Each input line is a JSON event in DING's normal shape: a `metric` field for matching, a `value` field for numeric conditions, and any other key/value pairs as labels (string) or floats (number). An optional `timestamp` field (RFC3339 string or Unix epoch number) controls the event's time for windowed rules; events without `timestamp` get sequential synthetic times starting from now.

Output format auto-detects: human-readable text when stdout is a terminal, JSON (one object per line) when piped. Override with `--format text|json`. Disable color with `--no-color`.

End-of-run rules (`mode: end-of-run`) fire after the last input event.

### `ding run --dry-run` — wrap a real workload, suppress sends

Same as `ding run`, but the dispatch boundary is swapped for a logging one — your wrapped command runs normally, events flow through the engine normally, the synthetic `run.exit` event still emits, end-of-run rules still fire, the wrapped command's exit code still propagates. Only `notifier.Send` is bypassed.

```sh
# Preview what alerts would fire on a real failing build
ding run --dry-run --config ding.yaml -- pytest tests/

# JSON output for piping
ding run --dry-run --format json --config ding.yaml -- ./train.sh | jq
```

Preview output goes to stderr (so the wrapped command's own stdout/stderr is uncontaminated by alert lines).

---

## Platform-specific examples

See [Recipes](recipes/index.md) for end-to-end configurations on specific CI/CD platforms (GitLab CI, Jenkins, Buildkite). Each recipe shows the auto-captured labels and the minimal `ding.yaml` for that platform.
