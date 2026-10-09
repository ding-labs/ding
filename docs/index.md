# Ding: persistent watches

Ding runs conditions over HTTP, command and authenticated push sources. Its local
SQLite store keeps incident state, evidence, timers and notification retries
across process restarts. A developer or agent manages the same inspectable
manifest and versioned CLI.

The current checkout is a watch preview. The published v0.14.0 artifacts contain
the legacy runtime. See [installation](install.md) before following watch commands.
The [qualification progress](development/progress.md) records completed gates.

Start with [configuration](configuration.md) and the [examples](examples.md).
[The API](api.md) exposes the same lifecycle and inspection operations to tools.
[Legacy migration](legacy.md) explains supported conversion and intentional changes.

Unlike a scheduled agent prompt, a watch has explicit typed conditions, persistent
state, durable evidence, resource limits and retryable delivery. A language model
can help author the declaration; the daemon evaluates it without a model call.
Data integrations still require an available feed and credentials.
