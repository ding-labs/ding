# Notice a missing heartbeat

Save the following as `heartbeat.yaml` and validate it with the watch preview.
This example needs no external provider. It prints an alert if ten minutes pass
without fresh input and recovers when another observation arrives.

```yaml
{{ snippet:examples/watches/missing-heartbeat.yaml }}
```

[Download the manifest](../assets/examples/missing-heartbeat.yaml).

```sh
./ding validate heartbeat.yaml --json
./ding apply heartbeat.yaml --state-dir ./ding-state --dry-run --json
./ding apply heartbeat.yaml --state-dir ./ding-state
```

Send `{"alive":true}` to `/v1/ingest/heartbeat` using the ingest token, as in the
[push guide](push.md). The ungrouped deadline starts when applied. Unknown input
does not postpone it; HTTP 304 can count as freshness for an HTTP missing-data watch.

The deadline persists through restart and fires once until recovery. Pausing
removes deadlines while preserving incident state; resuming starts fresh deadlines.
For a grouped watch, a deadline begins only after each entity is first observed.
Ding cannot detect an entity it has never seen.
