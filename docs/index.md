# DING

> Don't store it. Stream it. DING it.

DING is a stream-based alerting daemon. Pipe metrics into it. It evaluates rules. It fires alerts. That's it.

**Single binary. No database. No agents. No cloud account.**

| Metric | Result |
|--------|--------|
| Alert latency p50 | **4ms** (Prometheus default: ~62s) |
| Requests / second | **116k** |
| Cold start p50 | **9ms** |
| Binary size | **~5MB** |

---

## Quickstart

**1. Install**

```bash
brew install zuchka/tap/ding
# or
curl -sf https://start.ding.ing | sh
```

**2. Create a config**

```bash
cp ding.yaml.example ding.yaml
ding validate
```

**3. Start DING**

```bash
ding serve
```

**4. Send an event**

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{"metric":"cpu_usage","value":97,"host":"web-01"}'
```

If the value triggers a rule, the alert fires immediately. No scrape interval. No pipeline delay.

**5. Or just pipe**

```bash
your-app | ding serve
```

DING detects piped stdin automatically and processes it alongside the HTTP server.

---

## Next steps

- [Install →](install.md) — Homebrew, binary script, Docker
- [Configuration →](configuration.md) — full YAML reference
- [HTTP API →](api.md) — ingest, rules, reload, metrics
- [Examples →](examples.md) — Slack alerts, windowed conditions, jq transforms
