# Inspection, replay, and agent contract

These commands use the same versioned local API as agents. The watch preview
binary is `ding`; its declaration and API version remain `ding.ing/v1alpha1`.

## Responses and authentication

Every API response is JSON: `{"apiVersion":"ding.ing/v1alpha1","data":...}` on
success or `{"apiVersion":"ding.ing/v1alpha1","error":{"code":"...","message":"..."}}`
on failure. Unknown routes and unsupported methods return `not_found` JSON.
CLI `--json` writes success to stdout and errors to stderr and exits nonzero.
Human text is not a parsing contract. All inspection, export, backup and retry
endpoints require the admin token. The ingest token cannot read them. `/health`
is an unauthenticated liveness check; `/v1/doctor` is diagnostic readiness data.

| CLI | API | Data |
| --- | --- | --- |
| `events --watch ID --cursor TOKEN --limit 100 --json` | `GET /v1/events?watch=ID&cursor=TOKEN&limit=100` | `events`, `cursor`, `more` |
| `events inspect EVENT_ID --json` | `GET /v1/events/EVENT_ID` | immutable `definition`, `event`, optional `checkpoint`, `replayStatus` |
| `events observations EVENT_ID --after 123 --json` | `GET /v1/events/EVENT_ID/observations?after=123` | `observations`, `after`, `more` |
| `watch inspect ID --entities-after KEY --deliveries-before 123 --json` | `GET /v1/watches/ID?entitiesAfter=KEY&deliveriesBefore=123` | `watch`, entity/delivery summaries and pagination fields |
| `doctor --json` | `GET /v1/doctor` | store, usage, limits, workers, source state, credential presence, delivery counts |
| `export --watch ID` | `GET /v1/watches/ID/export` | API: `manifest`; default CLI: reusable multi-document YAML |
| `backup --out /absolute/new.db --json` | `POST /v1/backup`, `{"path":"/absolute/new.db"}` | `path`, `verified` |
| `delivery inspect ID --before 123 --json` | `GET /v1/deliveries/ID?before=123` | immutable intent/payload, newest 100 attempts, `before` |
| `delivery retry ID --json` | `POST /v1/deliveries/ID/retry` | ID and pending status |
| `replay evidence.json --json` | offline only | event ID and `verified` |

IDs in URL paths and cursors in query strings must be URL encoded. `limit` is
1–1000. Event pages also stop at 8 MiB. Inspection returns 100 entity summaries
and 100 delivery summaries, with `entitiesAfter`/`entitiesMore` and
`deliveriesBefore`/`deliveriesMore`; no bulk state windows or payloads are loaded.
Delivery detail includes the single pinned payload (base64 in JSON), not secrets
resolved at send time. Attempt pages are newest first; request the next `before`
until an empty page. Observation evidence pages stop at 100 records or 4 MiB.
Every exported observation has its actual committed sequence number.

## Resuming history

Cursors are opaque and scoped to the persistent store identity and optional watch
filter. Reusing a cursor for another store or filter returns `invalid_cursor`
(400). Retention tracks the highest removed sequence per watch and globally,
including non-contiguous deletion around pinned incident evidence. If a cursor
would skip a removed event, the API returns `cursor_expired` (410). Starting
without a cursor reads whatever remains; it does not assert complete history.
Once consumed, deletion of an older event does not invalidate a later cursor.
Backups preserve store identity. A cursor newer than restored data is invalid.
The earlier alpha numeric `after` event offset is rejected with migration advice.

`events --follow --json` polls once per second when caught up, emitting JSONL
pages including empty heartbeat pages with the latest cursor. Full pages drain
immediately. Persist the cursor after consuming its page. Interruption stops the
client, not the daemon. Expiry exits with an error; it never silently restarts.

## Replay checkpoints and schema 2

Schema 2 adds event checkpoints and cursor expiry tracking. An existing schema 1
store receives a verified pre-upgrade backup before the transactional migration.
Each evaluated event commits its relevant prior entity state and triggering observation in
the same transaction as observations, event and delivery intents. The event's
immutable definition remains pinned. Checkpoints expire only with their event.

`events inspect` re-evaluates the checkpoint and compares the complete event,
including ID, time, fields, message and evidence sequences (excluding its database
sequence). It reports `verified` only after equality. `replay` performs the same
check entirely offline, accepting either the JSON envelope or raw evidence data.
For new-event firing, unrelated dedup IDs are omitted once available capacity is
confirmed; this preserves the event decision without quadratic history growth.
Checkpoint verification is reproducibility, not a cryptographic authenticity
claim. A checkpoint is not a replay of the entire source history. Referenced
selected observations are available through the paginated evidence endpoint.
Lifecycle events and events written before schema 2 report unavailable replay;
Ding never invents their prior state. Evidence files may contain selected source
fields and should be handled like the underlying alert data.

The client bounds responses/evidence files at 64 MiB, enough for the declared
100,000-sample/ID maximum checkpoint. Ordinary event/observation/inspection pages
are much smaller. Numeric windows and dedup maps increase checkpoint storage;
store quotas still apply atomically and reject input rather than lose evidence.

## Diagnostics, exports, backup, retry

Doctor performs no source request or delivery. It reports SQLite integrity/WAL
bytes, live pages and row counts, configured limits, acquisition usage and worker
limits, per-source lifecycle/health, and open incident counts. Credential checks
report only environment variable names and whether a nonempty value is present.
`lastError` is a redacted runtime error that makes readiness unhealthy until a
new durable input commits successfully. A duplicate receipt does not prove write
recovery. Readiness also follows store integrity, quotas, source errors, missing
credentials and terminal failures.
A stopped/closing runtime is not healthy. Health is a point-in-time diagnostic,
not a monitoring substitute. A firing condition is not itself a daemon failure.

Export includes the watch and its current referenced destinations. Already queued
deliveries retain their own destination revisions; inspect them separately.
Exports keep environment references and never resolve them. Literal manifests,
command argv, and selected fields must not contain secrets: arbitrary user text
cannot be reliably classified or redacted while preserving a runnable definition.

Backup uses SQLite VACUUM INTO, integrity checking, fsync and no-overwrite
publication. It serializes database operations while running, so use it during
low load. Its path is on the daemon host. Stop the daemon before restoring the
verified backup as `ding.db` in a new private state directory, then start against
that directory. Do not copy a live database without its WAL. Credentials are
separate from the database and are generated for a new state directory.

Manual retry is allowed only for permanent, exhausted or canceled deliveries.
It records an attempt-history entry, resets the policy cycle's attempts/age and
preserves original creation time, event ID, payload and destination revision.
The shared provider backoff still applies. Retrying pending, leased or delivered
work returns `delivery_not_terminal` (409). Quota and shutdown restrictions apply.
A canceled deleted watch's retained deliveries can be explicitly retried; this
can send the old notification. Remote receipt is at least once, never exactly
once. Missing records return `not_found`, quotas `quota_exceeded`, and unavailable
storage `store_unavailable`. A failed backup returns `backup_failed` and publishes
no partial backup.

## Authoring demonstration

The repository skill is [ding](https://github.com/ding-labs/ding/blob/codex/ding-watch-runtime/skills/ding-watch/SKILL.md). Install it
with the repository resources available, or use its self-contained instructions.
It performs no automatic mutation beyond the user's requested scope. No LLM or
provider SDK enters the runtime.

The request “check my health URL every five seconds, notify after three actual
5xx responses, and recover after two healthy responses” maps to
[api-health.yaml](https://github.com/ding-labs/ding/blob/codex/ding-watch-runtime/examples/watches/api-health.yaml). Its URL is an explicit
example; substitute the user's real endpoint and supply `OPS_WEBHOOK_URL` in the
daemon environment. The authoring workflow validates and explains the manifest,
replays [api-health.jsonl](https://github.com/ding-labs/ding/blob/codex/ding-watch-runtime/testdata/watches/api-health.jsonl), dry-runs apply,
applies, inspects, exports, and verifies an emitted event offline. The CLI
integration test executes this flow through an authenticated local API, checks
that dry-run changes nothing, and verifies the exported revision remains equal.
It does not claim an automated natural-language benchmark or invented data-feed
access. Percentage-change, sports, traffic and airfare requests require an
available feed and any unsupported computation upstream.
