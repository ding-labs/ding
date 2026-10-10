# Ding

Persistent watches and durable alerts for developers and agents.

Ding watches for conditions you care about and tells you when they fire or
recover. Poll an HTTP endpoint, run a local command, or push JSON from your own
systems. Define each watch in YAML, then let a local daemon evaluate it, retain
the evidence, and deliver alerts—even across restarts.

Use it to alert after three failed health checks, detect a changed value, catch
missing data, or notify on a new provider event. Developers and agents use the
same versioned manifests, CLI, and local API. Checks run without a model call.

**Current status: watch preview.** The `main` branch contains the new watch
runtime. The latest published release,
[v0.14.0](https://github.com/ding-labs/ding/releases/tag/v0.14.0), contains the legacy
job-wrapper runtime. Build from source below to try watches. A stable watch
release has not been declared; see the [release checklist](docs/releases/watch-checklist.md)
and [qualification progress](docs/development/progress.md).

The monorepo also includes an [official Go SDK MCP integration for ChatGPT and Claude](docs/integrations/README.md)
with embedded views and plugin packaging. It is under qualification; official
marketplace publication has not yet happened.

## What you can watch

| Part | Supported capabilities |
| --- | --- |
| Sources | HTTP polling, explicit local commands returning JSON, authenticated JSON push |
| Conditions | Typed comparisons, numeric aggregates and windows, value changes, missing data, new provider IDs |
| Alert policies | Fire on transitions or at configured intervals; consecutive checks, recovery counts, cooldowns, and provider-ID deduplication |
| Destinations | Console, webhooks, Slack incoming webhooks, Discord webhooks |
| Local state | SQLite stores condition state, timers, retained evidence, events, and pending deliveries |

A watch declares its source, condition, alert policy, and destinations. Ding
validates the manifest before applying it. You can inspect a running watch,
pause or resume it, and replay retained event evidence offline.

Start with the [HTTP health](examples/watches/api-health.yaml),
[command](examples/watches/command.yaml), or
[provider event](examples/watches/provider-events.yaml) examples. Replace example
URLs and command paths with real sources. External data feeds require your own
provider access and credentials.

## Try a local watch

Requires Go 1.26, Git, curl, and jq. In a terminal:

```sh
git clone https://github.com/ding-labs/ding.git
cd ding
go build -o ding ./cmd/ding
./ding daemon --state-dir ./ding-state
```

Leave the daemon running. In another terminal, open the same `ding` directory.
The [example manifest](ding.yaml.example) creates a push watch named `latency`
that fires above 300 ms and sends firing and recovery alerts to the console.
Validate it, preview the changes, and apply it:

```sh
./ding validate ding.yaml.example --json
./ding apply ding.yaml.example --state-dir ./ding-state --dry-run --json
./ding apply ding.yaml.example --state-dir ./ding-state
```

Send a high reading to fire the watch:

```sh
curl --fail-with-body http://127.0.0.1:7676/v1/ingest/latency \
  -H "Authorization: Bearer $(jq -r .ingest ding-state/tokens.json)" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: quickstart-high' \
  -d '{"latency_ms":350}'

./ding events --watch latency --state-dir ./ding-state --json
./ding watch inspect latency --state-dir ./ding-state --json
```

The daemon prints one firing alert. Further high readings keep the incident open
without firing again. Send a low reading to recover:

```sh
curl --fail-with-body http://127.0.0.1:7676/v1/ingest/latency \
  -H "Authorization: Bearer $(jq -r .ingest ding-state/tokens.json)" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: quickstart-recovery' \
  -d '{"latency_ms":100}'

./ding events --watch latency --state-dir ./ding-state --json
```

Use a new idempotency key for each new reading. Reuse a key only when retrying the
same payload. The quickstart keys above are intended for a fresh state directory.

Stop the daemon with Ctrl+C and restart it with the same state directory to retain
condition state and pending deliveries. Keep that directory private: it contains
API credentials and retained observation fields and must never be committed.

## Manage and inspect watches

Use the same state directory to connect to the running daemon:

```sh
./ding watch list --state-dir ./ding-state --json
./ding watch pause latency --state-dir ./ding-state
./ding watch resume latency --state-dir ./ding-state
./ding doctor --state-dir ./ding-state --json
./ding export --watch latency --state-dir ./ding-state
./ding backup --out "$PWD/ding-backup.db" --state-dir ./ding-state
```

The CLI and [local API](docs/api.md) expose watch lifecycle, event evidence, and
delivery inspection. Most inspection commands accept `--json` for tools and
agents. The [Ding watch authoring skill](skills/ding-watch/SKILL.md) guides agents
through supported capabilities, validation, and normal lifecycle commands.

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

Ding needs a running daemon and a persistent local state directory. HTTP defaults
to loopback with separate admin and ingest credentials. Command sources run as
the daemon user and are trusted configuration, not a sandbox. See
[installation and operation](docs/install.md) for container and service-manager
guidance.

## Coming from legacy Ding?

The new runtime replaces the job-wrapper model with persistent watches. Legacy
`run`, `serve`, `test-rule`, and `install` commands now provide migration guidance.
Keep [v0.14.0](https://github.com/ding-labs/ding/releases/tag/v0.14.0) for existing
legacy workloads, or convert a supported configuration:

```sh
./ding migrate --config old.yaml --out converted --json
```

The converter writes manifests and a per-rule report into a new directory.
Unsupported rules are reported explicitly; conversion does not start watches or
import legacy state. Review the [migration guide](docs/legacy.md) before applying
the results.

## Documentation and development

- [Configuration](docs/configuration.md) and [examples](docs/examples.md)
- [Ding Console](docs/console/index.md): watch state, event evidence, delivery history, and reviewed changes
- [Local API](docs/api.md) and [inspection/replay contracts](docs/development/inspection-contract.md)
- [Watch preview release notes](docs/releases/watch-preview.md) and [release qualification](docs/development/qualification.md)

Build the browser console with Go 1.26 and Node 24, then open it from a second terminal:

```sh
make console
./ding daemon
# In a second terminal:
./ding ui
```

The console is embedded in the binary. Node is only needed to build it. Ordinary Go builds stay headless.

Run the development checks with Go 1.26:

```sh
go test -race ./...
go vet ./...
```

Ding is self-hosted and licensed under [Apache-2.0](LICENSE).
