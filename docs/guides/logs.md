# Monitor application output

Ding counts detected warnings and errors from ordinary text or JSON, publishing
one summary per second. Applications do not need a Ding SDK or JSON logging.
Collection and counting run locally; no model processes individual lines.

In the Console, open **Workbench → Log setup**, choose an input and rule, try a
sample, then review and apply the generated draft. Custom parsing patterns and
multiline settings can be added in the manifest editor and previewed there.

## Capture stdout and stderr

Start the [local daemon](../operate/local-setup.md), then apply this example:

```yaml
{{ snippet:examples/watches/log-warnings.yaml }}
```

```sh
ding apply examples/watches/log-warnings.yaml --dry-run
ding apply examples/watches/log-warnings.yaml
ding logs exec --watch application-logs -- your-command --your-flag
```

This example delivers to the daemon's console. Select an existing desktop,
webhook, or other [destination](notifications.md) for notifications.

`logs exec` inherits your environment, working directory, and stdin. It forwards
stdout and stderr separately, preserving bytes, and returns the application's
exit status. Put Ding flags **before** `--`. Signals are forwarded to the owned
process group, with five seconds for shutdown before forced termination. Final
pipe draining is bounded when descendants retain handles. Monitoring failures
after launch leave the application running and produce a diagnostic on stderr.

For an existing output stream:

```sh
your-command 2>&1 | ding logs ingest --watch application-logs
docker logs --follow --tail 0 my-app 2>&1 | ding logs ingest --watch application-logs
```

Piped input is forwarded to stdout. EOF ends the capture session; Ding does not
supervise the upstream application. A merged pipe cannot retain stdout/stderr
identity. There is no generic attach-to-PID mode: connect to the application's
existing file, container log stream, or supervisor.

These modes run in the foreground. Applying an external watch launches nothing.
Pause disables evaluation while the wrapper keeps forwarding output; resume or
a daemon restart causes a fresh session and warmup. Changed collector settings
require restarting the capture explicitly. Ding never relaunches a foreground
application. Pipes can change buffering and terminal/color detection; use the
application's unbuffered logging option when needed. PTYs and full-screen
interactive programs are outside this implementation.

## Understand the percentage

The example fires when **detected warnings / all nonblank lines > 50%** in the
last complete minute, provided at least 100 lines were collected. It recovers
after three consecutive ready summaries at or below 50%. Those summaries have
overlapping windows. Use `error_match_ratio` for `(errors + fatal) / all lines`.

Unclassified text stays in the denominator. It is not assumed to be healthy.
The Console shows classification coverage and the underlying counts; set
`minCoverage: 0.8`, for example, to require recognized severity on 80% of records.
Severity is never inferred solely from stderr. A line such as `0 errors` is
unclassified rather than an error.

Windows count records when the reader completes them, ignoring application
timestamps. A snapshot at second boundary `T` covers `[T-window, T)`. Blank lines
are ignored; a final line without a newline is counted at EOF. A short-lived
command may end before any complete window is eligible. The final partial
second is not promoted into a completed window or evaluated as a recovery.

Warmup, low traffic, and low coverage are **waiting** states. Parse failures,
oversized records, dropped analysis, and lost collection continuity make the
source unhealthy. Both states hold an open incident and break pending streaks.
An idle reader still sends summaries. Missing summaries are detected within
five seconds while the daemon is running. EOF is recorded as collection ended,
with an application exit code where available; it does not imply recovery.

## Choose parsing and framing

`format: auto` recognizes a configured JSON severity field, or common text
headers such as `WARN`, timestamped `ERROR`, and `level=info`. `format: text`
uses only text rules; `format: jsonl` requires one valid JSON object per line.
Customize `levelField`, including dotted paths such as `log.level`. JSON strings
use the common severity names; numeric or custom labels require an explicit map:

```yaml
levelMap: {'10': trace, '20': debug, '30': info, '40': warning, '50': error, '60': fatal}
```

For application-specific text headers, use explicit patterns:

```yaml
format: text
patterns:
  warning: '^W\d{4} '
  error: '^E\d{4} '
```

Patterns use Go regular expressions. Highest severity wins among overlapping
custom patterns. Unmatched lines fall back to the common header profile.
ANSI sequences and CRLF are normalized only in the analysis copy. Choose
`framing: carriage-return` to count carriage-return progress updates separately.

For stack traces, explicitly group continuation lines:

```yaml
format: text
framing: multiline
multiline:
  start: '^(ERROR|WARN|INFO) '
  continue: '^\s'
  maxLines: 100
  flushAfter: 1s
```

Grouping is per stream. A new start, a non-continuation, EOF, or the flush
deadline completes a record. Multiline records share the 64 KiB limit and allow
2–1,000 lines and a 100 ms–5 s deadline. An oversized record is drained and
counted once as invalid. Without multiline framing the denominator is **lines**.
The sample preview sends at most 64 KiB to the local daemon and retains no text.

## Run a reader in the daemon

Replace `source.logs.input` with a file input:

```yaml
input:
  type: file
  path: /absolute/path/application.log
  start: end
```

The daemon reads regular files only, starting at their current end. It checks
identity, size, and a bounded suffix to detect rotation and truncation. It drains
the old descriptor before switching at EOF, then skips the replacement's existing
contents and records a gap. Truncation, abandoned partial records, unreadable
files, or more than 4 MiB of unread backlog also invalidate evidence. This is
live monitoring, not lossless replay; rapid replacement or an undetectable
overwrite can remove bytes before a reader sees them. After interruption the
reader starts at the current end with fresh evidence.

Or configure an observer command:

```yaml
input:
  type: command
  argv: [/usr/local/bin/docker, logs, --follow, --tail, '0', my-app]
  directory: /absolute/working/directory
  reconnectEvery: 5s
  env:
    DOCKER_HOST: {env: DING_DOCKER_HOST}
```

The daemon executes explicit argv without inserting a shell, passing only
declared environment references. Choose a live-only log reader, not an
application-start command: it reconnects after exit, and pausing/removing the
watch stops its process group. Both output channels are analyzed; a reader CLI's
own diagnostics can therefore affect counts. Dedicated platform adapters are
not included. File failures retry after five seconds; command retry delays can
be configured between five seconds and five minutes.

There are eight daemon-reader slots, separate from poll and delivery workers.
Additional configured readers wait for a slot. These readers continue when the
Console or terminal closes, while the daemon remains running. Files and commands
use the daemon user's permissions. For MCP/integration grants, each exact local
reader revision must be explicitly allowed through `commandRevisions` (CLI
`--allow-command-revision`); environment references also need `secretRefs`.
Cloud transfer rejects log sources.

## Storage and operating limits

Ding stores summaries, session identities, and available file positions; it does
not store raw log text. Checkpoints, observations, evaluation, and alert outbox
writes commit together. Acknowledgement of a summary does not guarantee raw pipe
bytes survive a crash. A restarted collector rebuilds its window and preserves
open incidents and committed deliveries.

Ordinary log summaries and receipts default to `retention: 1h` (configurable from
`1m` to `168h`). Global history retention can expire summaries sooner. Incident
evidence and pending delivery references can remain longer. Other watches keep
their existing history policy. Each stream produces roughly 3,600 ordinary
summaries per hour.

Collection is supported on macOS and Linux; Windows capture is rejected pending
platform qualification. Record size is 64 KiB, windows are 1–3,600 whole seconds,
and analysis queues hold at most 4,096 records / 4 MiB. Queue overflow is counted
and invalidates evidence. Forwarding retains normal downstream backpressure: a
blocked output destination can still block the application.

See [log qualification](../development/log-qualification.md) for measured runs
and limits that remain unqualified. For experimental compatibility, `logs exec`
and `logs ingest` can also target push watches with explicit parser/window flags;
native log watches provide the readiness, session, and lifecycle behavior above.
