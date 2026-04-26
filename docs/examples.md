# Examples

## Slack alert on CPU threshold

Send an alert to Slack when CPU exceeds 95% on any host.

```yaml
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
    condition: value > 95
    cooldown: 5m
    message: "CPU spike on {{ .host }}: {{ .value }}%"
    alert:
      - notifier: slack
```

DING posts a Block Kit message to Slack with the rule name as a header, the message as body text, and metric/value/labels as structured fields. Use `type: webhook` if you need a raw JSON payload instead.

---

## CI job failure alert to Slack

Alert to Slack when a CI job exits non-zero. Run context (branch, commit, exit code, duration) is surfaced automatically — no template work needed.

```yaml
notifiers:
  slack:
    type: slack
    url: https://hooks.slack.com/services/T.../B.../...

rules:
  - name: job_failed
    match:
      metric: run.exit
    condition: value > 0
    message: "Job failed with exit code {{ .value }}"
    alert:
      - notifier: slack
      - notifier: github_actions
```

Run it:

```bash
ding run --config ding.yaml -- pytest tests/
```

When the job exits non-zero, Slack receives a Block Kit message with exit code, duration, branch, commit, and run ID fields populated from the CI environment automatically. The `github_actions` notifier simultaneously writes a `::warning::` annotation and a step summary entry.

---

## CI job failure alert to Discord

Alert to Discord when a CI job exits non-zero. Run context (branch, commit, exit code, duration) is surfaced automatically as embed fields — no template work needed.

```yaml
notifiers:
  discord:
    type: discord
    url: https://discord.com/api/webhooks/WEBHOOK_ID/WEBHOOK_TOKEN

rules:
  - name: job_failed
    match:
      metric: run.exit
    condition: value > 0
    message: "Job failed with exit code {{ .value }}"
    alert:
      - notifier: discord
```

Run it:

```bash
ding run --config ding.yaml -- pytest tests/
```

When the job exits non-zero, Discord receives an embed with the rule name as the title, exit code, duration, branch, commit, and run ID populated from the CI environment automatically.

---

## Windowed average condition

Fire only when CPU has been high for 5 sustained minutes, not on a single spike.

```yaml
rules:
  - name: cpu_sustained
    match:
      metric: cpu_usage
    condition: avg(value) over 5m > 80
    cooldown: 10m
    message: "Sustained high CPU on {{ .host }}: {{ .avg }}% avg over 5m"
    alert:
      - notifier: slack
```

DING maintains an in-memory ring buffer per label-set. No database required.

---

## Prometheus text format input

If your app already emits Prometheus exposition format, set `format: prometheus` or leave it as `auto`.

```yaml
server:
  port: 8080
  format: prometheus  # or auto
```

Send events:

```bash
curl -X POST http://localhost:8080/ingest \
  --data-binary @- << 'EOF'
cpu_usage{host="web-01",region="us-east"} 92.5
memory_usage{host="web-01"} 78.2
EOF
```

Label keys from Prometheus metric labels (`host`, `region`) are available in rule `match` and message templates.

---

## JQ inbound transform

If your app emits a custom JSON shape, use a `jq` filter to normalize it before rule evaluation.

```yaml
server:
  jq: '.events[] | {metric: .name, value: .reading, host: .tags.host}'
```

This accepts payloads like:

```json
{
  "events": [
    {"name": "cpu_usage", "reading": 97.2, "tags": {"host": "web-01"}},
    {"name": "cpu_usage", "reading": 88.1, "tags": {"host": "web-02"}}
  ]
}
```

The filter runs on every inbound request, producing one normalized event per output object. No changes to your rules required.

---

## Stdin pipe pattern

DING detects piped stdin automatically. This composes with any tool that writes JSON to stdout.

```bash
# Pipe from a custom metrics emitter
./my-metrics-emitter | ding serve

# Replay a log file
cat events.jsonl | ding serve --config ding.yaml

# Generate synthetic load for testing
while true; do
  echo '{"metric":"cpu_usage","value":'$((RANDOM % 100))',"host":"test"}'
  sleep 0.1
done | ding serve
```

The HTTP server stays fully active while stdin is being read. EOF on stdin does not stop the daemon.

---

## Alert to multiple notifiers

Fire the same alert to both Slack and PagerDuty simultaneously.

```yaml
notifiers:
  slack:
    type: slack
    url: https://hooks.slack.com/services/...
  pagerduty:
    type: webhook
    url: https://events.pagerduty.com/v2/enqueue
    max_attempts: 5
    initial_backoff: 2s

rules:
  - name: disk_full
    match:
      metric: disk_usage
    condition: value > 90
    cooldown: 30m
    message: "Disk usage critical on {{ .host }}: {{ .value }}%"
    alert:
      - notifier: slack
      - notifier: pagerduty
      - notifier: stdout
```

`stdout` requires no declaration and is always available.

---

## Hot-reload config

Update rules without restarting:

```bash
# Edit ding.yaml, then:
kill -HUP $(pgrep ding)

# Or via HTTP:
curl -X POST http://localhost:8080/reload
```

If the new config is invalid, DING logs the error and keeps the current config active.

---

## Persist state across restarts

Keep cooldown history and windowed buffers after a restart or redeploy.

```yaml
persistence:
  state_file: /var/lib/ding/state.json
  flush_interval: 30s
```

DING restores from the snapshot on startup. If the file doesn't exist yet, it starts fresh with no error.
