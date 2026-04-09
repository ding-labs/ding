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
    type: webhook
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

---

## `notifiers`

A map of named notifiers. Reference them by name in rule `alert` blocks.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | — | `webhook` or `stdout` |
| `url` | string | — | Webhook destination URL (required for `type: webhook`) |
| `max_attempts` | int | `3` | Total attempts including the initial request (webhooks only) |
| `initial_backoff` | duration | `1s` | First retry delay; doubles each attempt (webhooks only) |

`stdout` is always available without declaration. 4xx responses from webhooks are dropped. 5xx responses are retried with exponential backoff.

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
