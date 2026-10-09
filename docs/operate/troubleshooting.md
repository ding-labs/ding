# Diagnose a watch or daemon

```sh
./ding doctor --state-dir ./ding-state --json
./ding watch inspect WATCH_ID --state-dir ./ding-state --json
```

Doctor checks the store, quotas, workers, source state, credential presence, and
delivery counts. It performs no source requests or notification delivery. It is a
point-in-time diagnostic; do not poll its integrity checks every second.

| Finding | Next step |
| --- | --- |
| Waiting for first input | Confirm a running lifecycle, source configuration, and producer authentication. |
| Open incident with unknown input | Inspect the source or projection; unknown data does not establish recovery. |
| Source error | Check endpoint reachability, command permissions/output, selected fields, timeout, and referenced environment names. |
| Missing credential | Set the named environment variable on the daemon host and restart deliberately. |
| Store or quota pressure | Inspect disk/WAL use, pinned evidence, pending delivery, and configured limits before adjusting capacity. |
| Permanent or exhausted delivery | Inspect the destination revision and recorded attempt, correct the cause, then explicitly retry if appropriate. |
| Expired event cursor | Acknowledge a history gap and read available history with a fresh cursor. |
| Revision conflict | Read the current definition, compare it with the draft, and dry-run again. |

An HTTP endpoint returning 503 can be successfully acquired data. A receiver's
429 is a delivery outcome. Neither means the same thing as a disconnected browser.
[State glossary](../reference/states.md) · [API errors](../api.md).
