# HTTP runtime milestone

The experimental `ding` daemon runs applied HTTP watches with a durable
SQLite outbox. The shipped legacy `ding` command remains unchanged until cutover.
Run the temporary binary from the repository:

```sh
go build -o /tmp/ding ./cmd/ding
OPS_WEBHOOK_URL=https://your-receiver.example/events \
  /tmp/ding daemon --state-dir ./ding-state
/tmp/ding apply examples/watches/api-health.yaml --state-dir ./ding-state --dry-run --json
/tmp/ding apply examples/watches/api-health.yaml --state-dir ./ding-state --json
/tmp/ding watch inspect api-health --state-dir ./ding-state --json
```

Replace the example source URL with an endpoint you control. The daemon resolves
secret references from its environment at I/O time. The CLI never embeds those
values in the manifest or database. The default listener is `127.0.0.1:7676`.
Separate generated admin and ingest tokens live in the private state directory.
Unix token permissions are checked; Windows relies on the user's directory ACL.
Remote listeners require `--allow-remote` and a TLS reverse proxy. The connection
file advertises the local endpoint for CLI use; it contains no token.

The scheduler permits 32 concurrent acquisitions, with at most one in flight for
each watch. It schedules the next poll after acceptance, performs one catch-up
poll after downtime, and does not fabricate missed historical checks. HTTP
redirects are rejected. Responses are bounded, and only HTTP status plus selected
scalar JSON fields are retained. Timeouts and invalid data become unknown
observations. Conditional requests persist ETag/Last-Modified only with the
observation transaction; 304 advances freshness without creating another sample.
Provider Retry-After delays the next source poll.

Acceptance atomically persists the observation, source checkpoint, condition
state, event, delivery intent, and input receipt. Poll identity derives from the
persisted generation and scheduled time. Repeating a committed input returns its
receipt for 24 hours. Failed evaluation or persistence rolls back the entire
batch; it cannot advance the cursor or enqueue half a batch.

Eight delivery workers claim 30-second leases and make attempts with a ten-second
timeout. Retry uses bounded exponential delay plus positive jitter and honors
Retry-After. Destination revision and payload are pinned when the event commits;
current values of the referenced secrets are resolved at attempt time. Jobs
remain ordered per watch/destination, while other pairs proceed independently.
Defaults are eight attempts and a 24-hour delivery age. Missing credentials,
permanent rejection, and exhaustion are terminal, inspectable outcomes.

Delivery is at least once. If a receiver accepts a request and the process dies
before recording success, recovery can send the same event again. Webhooks carry
`Idempotency-Key` and `X-Ding-Event-Id` for receiver deduplication. Shutdown stops
acquisition, waits for its canceled requests, and gives committed delivery work
up to five seconds to drain. Remaining durable jobs resume on the next start.

The P09 gate includes a real five-second polling test that observes three 503s,
records a firing and 429 retry, kills the process, reopens the store, then observes
two healthy responses and delivers firing followed by recovery. A separate test
exercises a reclaimed lease and rejects the old worker's acknowledgment. Offline
fixture replay verifies the same condition transitions without source I/O.

P10 adds revision replacement, pause/resume/delete, missing-data timers, and
retention/backpressure. See [the lifecycle contract](lifecycle-contract.md).
