# Create your first watch

This walkthrough uses the [watch source preview](../install.md), curl, and jq.
Run it from the source checkout in two terminals. It sends no external notification:
the destination is the daemon's console output.

## Start the daemon

In the first terminal:

```sh
./ding daemon --state-dir ./ding-tutorial
```

Keep it running. The default address is `127.0.0.1:7676`. If another daemon owns
that port, stop that daemon deliberately or choose a different `--listen` port
and update the requests below. Do not reuse an existing production state directory.

## Validate and apply

In the second terminal, from the same checkout:

```sh
./ding validate ding.yaml.example --json
./ding apply ding.yaml.example --state-dir ./ding-tutorial --dry-run --json
./ding apply ding.yaml.example --state-dir ./ding-tutorial
```

Validation performs no I/O. Dry-run reports changes without saving them. Applying
creates the watch and destination. [Download the same manifest](../assets/examples/ding.yaml.example).

```yaml
{{ snippet:ding.yaml.example }}
```

## Send a high reading

```sh
curl --fail-with-body http://127.0.0.1:7676/v1/ingest/latency \
  -H "Authorization: Bearer $(jq -r .ingest ding-tutorial/tokens.json)" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: tutorial-high' \
  -d '{"latency_ms":350}'

./ding events --watch latency --state-dir ./ding-tutorial --json
./ding watch inspect latency --state-dir ./ding-tutorial --json
```

A successful push returns a durable receipt. The event list contains a `firing`
event, and the first terminal prints a notification. Another high reading keeps
the incident open without repeating the transition alert.

The ingest token authorizes input, not administration. Do not substitute the
admin token, paste a real token into documentation, or save it in the manifest.

## Record recovery

```sh
curl --fail-with-body http://127.0.0.1:7676/v1/ingest/latency \
  -H "Authorization: Bearer $(jq -r .ingest ding-tutorial/tokens.json)" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: tutorial-recovery' \
  -d '{"latency_ms":100}'

./ding events --watch latency --state-dir ./ding-tutorial --json
```

The list now also contains `recovered`. The new idempotency key matters: reusing
`tutorial-high` returns the original receipt instead of accepting a new input.
Use fresh keys when repeating the walkthrough against an existing tutorial store.

## Restart and inspect

Stop the daemon with Ctrl+C, then start it again with the same state directory.
Read the events again: condition state, history, and pending deliveries persist.

```sh
./ding doctor --state-dir ./ding-tutorial --json
./ding watch pause latency --state-dir ./ding-tutorial
```

Pause stops acquisition; it does not cancel already committed delivery. Stop the
daemon when finished and keep or remove only your disposable tutorial directory.

## If something goes wrong

| Symptom | Check |
| --- | --- |
| Connection refused | Daemon is running; port and state directory agree. |
| Unauthorized | Request uses the ingest credential from this daemon's private directory. |
| No new event | Use a fresh idempotency key and inspect the watch; a transition fires once until recovery. |
| Unknown command or invalid manifest | You built the watch snapshot, not the v0.14.0 legacy binary. |
| Event exists but no notification | Inspect delivery status and daemon output; event creation and delivery are distinct. |

Next: [investigate evidence](investigate.md), [send notifications](notifications.md),
or [monitor an HTTP endpoint](http.md).
