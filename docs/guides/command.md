# Run a local check

A command source runs on the **daemon host** as the daemon user. It is trusted
configuration, not a sandbox. Choose a read-only or safely repeatable check.

```yaml
{{ snippet:examples/watches/command.yaml }}
```

[Download this manifest](../assets/examples/command.yaml). Replace the executable,
absolute working directory, and selected JSON path for your environment. The
program should write one JSON document, for example `{"service":{"healthy":false}}`.
Only declared environment references, plus required Windows process environment,
are passed to the child. No shell is inserted into `argv`.

```sh
./ding validate examples/watches/command.yaml --json
./ding explain examples/watches/command.yaml --json
./ding apply examples/watches/command.yaml --state-dir ./ding-state --dry-run --json
```

Review the execution permissions, then apply deliberately. This example records
events without a destination; add one using [notifications](notifications.md).
Timeout, nonzero exit, excess output, or invalid JSON produces unknown input and
a redacted error. [Doctor](../operate/troubleshooting.md) helps diagnose it.
