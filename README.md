# Ding

Persistent watches and durable alerts for developers and agents.

Describe a condition as a small versioned manifest. Ding polls an HTTP endpoint,
runs an explicit local command, or accepts authenticated JSON. It keeps condition
state across restarts, records why an event happened, and retries delivery through
a durable outbox. An agent can author and inspect the same manifest and CLI.

**This branch is the watch preview.** The published v0.14.0 release is the legacy
runtime. Build this checkout to try watches; release qualification is tracked in
[the implementation progress](docs/development/progress.md). A watch stable release
has not been declared. [Legacy users and migration](docs/legacy.md).

## Try a local watch

Requires Go 1.26 to build, plus curl and jq for this example.

```sh
go build -o ding ./cmd/ding
./ding daemon --state-dir ./ding-state
```

Leave the daemon running. In another terminal:

```sh
./ding validate ding.yaml.example --json
./ding apply ding.yaml.example --state-dir ./ding-state --dry-run --json
./ding apply ding.yaml.example --state-dir ./ding-state

curl --fail-with-body http://127.0.0.1:7676/v1/ingest/latency \
  -H "Authorization: Bearer $(jq -r .ingest ding-state/tokens.json)" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: quickstart-high' \
  -d '{"latency_ms":350}'

./ding events --watch latency --state-dir ./ding-state --json
./ding watch inspect latency --state-dir ./ding-state --json
```

The daemon prints one firing event to its console. Send a reading of 100 using a
new idempotency key to record recovery. Restart the daemon with the same state
directory to keep state and pending deliveries. Never commit the state directory:
it contains private API credentials and retained observation fields.

The [example manifest](ding.yaml.example) is a push watch with a console
destination. [HTTP health](examples/watches/api-health.yaml),
[command](examples/watches/command.yaml), and
[provider event](examples/watches/provider-events.yaml) examples are also included.
Command paths and example URLs must be replaced with your real source.

## What the runtime guarantees

- Observations, condition state, events, and delivery intents commit together.
- Transition alerts, recovery counts, cooldowns, missing-data deadlines, typed
  value changes, and bounded provider-ID deduplication survive restart.
- Unknown/failed input is visible; it does not silently clear an open incident.
- Webhook, Slack and Discord attempts share bounded retries and durable backoff.
  Remote delivery is at least once; receivers should deduplicate stable event IDs.
- Retained evidence can be replayed offline. Cursor expiry explicitly reports a
  history gap. Resource limits apply backpressure instead of silently approximating
  a full rolling window.

Ding needs a running daemon. An AI model is not invoked on each check. The
[authoring skill](skills/ding-watch/SKILL.md) translates a request into the declared
capabilities, validates it, and uses normal lifecycle commands. Sports, airfare,
traffic, and financial feeds require real provider access; there are no bundled
consumer data feeds, trading actions, or arbitrary autonomous workflows.

## Develop and operate

```sh
go test -race ./...
go vet ./...
./ding doctor --state-dir ./ding-state --json
./ding export --watch latency --state-dir ./ding-state
./ding backup --out "$PWD/ding-backup.db" --state-dir ./ding-state
```

Read [configuration](docs/configuration.md), [the local API](docs/api.md),
[migration](docs/legacy.md), and [inspection/replay contracts](docs/development/inspection-contract.md).
HTTP defaults to loopback with separate admin and ingest credentials. Command
sources run as the daemon user and are trusted configuration, not a sandbox.
The Apache-2.0 licensed core is self-hosted; no hosted service or billing is included.
