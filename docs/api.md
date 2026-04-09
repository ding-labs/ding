# HTTP API

DING exposes five HTTP endpoints. Default port: `8080`.

---

## POST /ingest

Send events for rule evaluation. Accepts JSON lines, Prometheus text format, or auto-detected input.

**Request**

```
POST /ingest
Content-Type: application/json
```

**JSON lines format** (one event per line):

```json
{"metric": "cpu_usage", "value": 92.5, "host": "web-01"}
{"metric": "cpu_usage", "value": 88.0, "host": "web-02"}
```

**Prometheus text format:**

```
cpu_usage{host="web-01"} 92.5
cpu_usage{host="web-02"} 88.0
```

With `server.format: auto` (default), DING detects the format per request. Set `server.format: json` or `server.format: prometheus` to pin it.

**Response**

```json
{"events": 2, "alerts_fired": 1}
```

**Error responses**

| Status | Meaning |
|--------|---------|
| `400` | Malformed payload |
| `413` | Body exceeds `max_body_bytes` |

**Example**

```bash
curl -s -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{"metric":"cpu_usage","value":97,"host":"web-01"}'
```

---

## GET /health

Liveness probe. Returns immediately with no side effects.

**Response**

```json
{"status": "ok"}
```

**Example**

```bash
curl -s http://localhost:8080/health
```

---

## GET /rules

Inspect active rules, their conditions, and current cooldown state.

**Response** — array of rule objects:

```json
[
  {
    "name": "cpu_spike",
    "condition": "value > 95",
    "cooldown": "1m0s",
    "cooling_down": {
      "host=web-01": true,
      "host=web-02": false
    }
  }
]
```

`cooling_down` is a map of label-set keys to boolean. `true` means the rule is suppressed for that label-set until the cooldown expires.

**Example**

```bash
curl -s http://localhost:8080/rules | jq
```

---

## POST /reload

Hot-reload the config file from disk without restarting the daemon. Equivalent to `kill -HUP <pid>`.

State is flushed to disk (if persistence is configured) before the new config is loaded, and restored into the new engine after.

If the new config is invalid, the reload fails and the current config remains active.

**Response (success)**

```json
{"status": "reloaded"}
```

**Response (failure)**

```json
{"error": "reload failed: config invalid: ..."}
```

**Example**

```bash
curl -s -X POST http://localhost:8080/reload
```

---

## GET /metrics

Prometheus-format metrics for observing DING itself.

**Response** — Prometheus text format (exposition format 0.0.4):

```
# HELP ding_events_total Total events ingested
# TYPE ding_events_total counter
ding_events_total 1042

# HELP ding_alerts_fired_total Total alerts fired
# TYPE ding_alerts_fired_total counter
ding_alerts_fired_total 3

# HELP ding_webhook_queue_depth Current webhook delivery queue depth
# TYPE ding_webhook_queue_depth gauge
ding_webhook_queue_depth 0
```

**Example**

```bash
curl -s http://localhost:8080/metrics
```

Scrape this endpoint from Prometheus to monitor DING's own throughput and alert rate.
