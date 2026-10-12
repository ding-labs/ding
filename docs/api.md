# Local control API

ChatGPT and Claude use the separate [scoped integration API](integrations/api.md)
with paired credentials, expiring previews, and transactional mutation receipts.

The daemon defaults to `127.0.0.1:7676`. Its private state directory contains
`tokens.json` (separate admin/ingest tokens) and `connection.json`. CLI clients read
these files without creating credentials. Use `--state-dir` consistently.

Responses use `ding.ing/v1alpha1` envelopes with either `data` or a stable
`error.code`. `/health` is public liveness. Control clients use an admin bearer token; the embedded console uses the
[bounded browser session](console/index.md) with CSRF protection. Only
`/v1/ingest/{watchId}` accepts the ingest token. The admin token cannot
be substituted on that route. Push returns 202 only after durable commit and
supports a bounded 24-hour `Idempotency-Key` receipt horizon.

Lifecycle routes are `POST /v1/apply`, `GET /v1/watches`,
`GET /v1/watches/{id}`, and `POST /v1/watches/{id}/lifecycle` with `action` of
pause, resume or delete. Application accepts a manifest, dry-run flag and expected
revision map. Delete retains history; canceling pending delivery is explicit.

See the [complete inspection API](development/inspection-contract.md) for event
cursors, evidence, doctor, export, backup, retry and bounded pagination.
[The HTTP runtime contract](development/http-runtime.md) covers atomic acceptance
and worker behavior. The daemon currently serves plaintext HTTP; remote binding
requires `--allow-remote` and your own TLS reverse proxy/access policy.

## Endpoint reference

Control paths accept the admin bearer token or an authenticated console session.
Cookie-authenticated mutations also require the configured Origin and CSRF header.
Ingestion requires its separate ingest token. IDs and query values must be URL encoded.

| Method and path | Operation |
| --- | --- |
| `POST /v1/apply` | Validate and apply a YAML bundle, or review with `dryRun`; guard edits with expected revisions. |
| `GET /v1/watches` | List applied watches. |
| `GET /v1/watches/{id}` | Inspect one watch, entities, and deliveries with bounded pagination. |
| `POST /v1/watches/{id}/lifecycle` | Pause, resume, or delete; canceling pending delivery is explicit. |
| `POST /v1/ingest/{id}` | Durably accept JSON with the ingest credential and optional idempotency key. |
| `GET /v1/events` | Read retained events with `watch`, `cursor`, and `limit`. |
| `GET /v1/events/{id}` | Inspect the event-time definition, checkpoint, and replay status. |
| `GET /v1/events/{id}/observations` | Page selected retained observations with `after`. |
| `GET /v1/watches/{id}/export` | Export a reusable manifest with destination references. |
| `GET /v1/deliveries/{id}` | Inspect a pinned delivery and attempt history with `before`. |
| `POST /v1/deliveries/{id}/retry` | Retry eligible terminal delivery work; duplicates are possible. |
| `GET /v1/doctor` | Run diagnostics without source acquisition or delivery. |
| `POST /v1/backup` | Create and verify a new backup at an absolute path on the daemon host. |

A `data` response is not proof of end-to-end notification receipt. For ingestion,
202 means the observation transaction committed. For retry, inspect the eventual
attempt result. Use the [inspection contract](development/inspection-contract.md)
for exact field shapes, cursors, page budgets, and backup semantics.

## Error handling

| Code or status | Meaning and response |
| --- | --- |
| `invalid_cursor` / 400 | Cursor is invalid for this store, query, or restored history. Start an intentional new read. |
| `cursor_expired` / 410 | Retention removed intervening history. Surface a gap before reading available history. |
| `delivery_not_terminal` / 409 | Work is queued, sending, or already delivered; it cannot be manually retried. |
| `quota_exceeded` | Capacity prevented acceptance; inspect limits and back off. |
| `store_unavailable` | Storage could not complete the operation; reconcile before retrying a mutation. |
| `not_found` | Route, method, or record is unavailable. |
| `backup_failed` | No partial backup is published. Diagnose the path, permissions, and storage. |

Use stable error codes rather than parsing human messages. A failed connection
can leave a mutation's outcome uncertain: inspect current state before repeating
it. The console-specific APIs and browser session flow below are available in
console-enabled source builds; published legacy binaries do not include them.

## Console additions

The console uses additive endpoints; existing CLI response shapes remain compatible:

- `GET /v1/info` and `/v1/status`: safe instance settings and cheap runtime status.
- `GET /v1/console/watches`, `/events`, `/deliveries`, `/destinations`: bounded read models (all four routes are under `/v1/console`). Defaults are 50 rows, maximum 100. Event history is newest first; existing `/v1/events` remains the forward cursor stream.
- `POST /v1/tools/compile`, `/test`, `/project`, `/replay`: pure, size-limited developer tools (all four under `/v1/tools`). They do not run sources or send notifications.
- `GET /v1/console/doctor`: explicit diagnostic work with an independent concurrency limit.
- `POST /v1/console/backup` and `GET /v1/console/backup/{id}`: a verified, expiring, single-use download owned by the requesting session. `POST /v1/backup` retains its host-path contract.
- `POST /v1/browser/handoff`: admin bearer only; obtains a one-use launch link. `POST`, `GET`, and `DELETE /v1/browser/session` establish, restore, and revoke a browser session.

Whole-bundle dry runs return review preconditions covering the store, exact manifest, watch revisions/generations/status, and destination revisions. Supplying this review on apply checks all of them in the write transaction. Review conflicts leave definitions untouched. Existing CLI revision checks remain supported.

The generated [TypeScript contracts](https://github.com/ding-labs/ding/blob/main/web/console/src/api/contracts.ts) are checked against Go types in CI. See [read consistency and browser boundaries](development/console-architecture.md) for cursor scope, retention invalidation and authentication details.

## Hosted push additions

The [cloud push contract](guides/push.md) uses `POST /v1/ingest/{watchId}` with a workspace/watch-scoped producer bearer token and required idempotency key. Browser, device, MCP, and local daemon tokens are not producer credentials. Hosted receipts bind to a watch lifetime and logical producer, include a body digest, and survive key rotation and watch edits. Local receipt semantics remain unchanged.

| Method / route | Permission and behavior |
| --- | --- |
| `GET /v1/cloud/config` | Public capability discovery; availability is refined per workspace in `/v1/info` |
| `GET /v1/services` / `POST /v1/services` | Owner/administrator metadata/issuance; ingest selects existing push watches |
| `PATCH /v1/services/{id}` | Owner/administrator; replace name, scopes, and watch IDs without rotating logical identity |
| `POST /v1/services/{id}/rotate` | Owner/administrator; `days` (1–90), `overlapSeconds` (0–86400); secret shown once |
| `GET /v1/services/{id}/keys` | Owner/administrator; key IDs, expiry, coalesced last-use metadata |
| `DELETE /v1/services/{id}/keys/{key}` | Owner/administrator; revoke one key |
| `DELETE /v1/services/{id}` | Owner/administrator; revoke all producer keys |
| `GET /v1/ingest-status/{watchId}` | Workspace inspection; durable last acceptance and bounded safe recent request diagnostics |
| `GET /v1/cloud/usage` | Workspace inspection; durable inbound request/byte/observation totals alongside outbound usage |
| `POST /v1/tools/project` | Preview permission; `manifest`, `watch`, `fixture` source JSON; pure projection and prediction, `persisted:false` |

The [optional native adapters](guides/cloud-webhooks.md) have separate signature-authenticated ingress and administrator connection routes. They are disabled by default. Cookie-authenticated setup retains ordinary CSRF/workspace checks. [Operations and capacity](development/cloud-push-operations.md) describe validated settings and release gates.
