# Watch an HTTP endpoint

Use a watch-preview binary from the source checkout. This example polls every
five seconds, opens an incident after three actual 5xx responses, and recovers
after two nonmatching observations. HTTP 4xx responses are nonmatching here;
choose a different condition if those should also open an incident.

```yaml
{{ snippet:examples/watches/api-health.yaml }}
```

[Download the complete manifest](../assets/examples/api-health.yaml). Replace the
example URL with your endpoint. Set `OPS_WEBHOOK_URL` in the daemon's environment
before starting it, or change the destination to console output for a local trial.

```sh
./ding validate examples/watches/api-health.yaml --json
./ding apply examples/watches/api-health.yaml --state-dir ./ding-state --dry-run --json
./ding apply examples/watches/api-health.yaml --state-dir ./ding-state
./ding watch inspect api-health --state-dir ./ding-state --json
```

## Test without a live service

```sh
./ding test examples/watches/api-health.yaml \
  --events testdata/watches/api-health.jsonl --json
```

The fixture returns one firing and one recovery. It performs no source I/O or
delivery. Its inputs are 500, 502, 503, 200, 200, five seconds apart.

A timeout is unknown input, not an HTTP 5xx observation. A successful 304 advances
freshness without counting as a new condition sample. Redirects are rejected.
[Source semantics](../development/adapter-contract.md) explain projection and limits.
