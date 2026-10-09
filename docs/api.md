# Local control API

The daemon defaults to `127.0.0.1:7676`. Its private state directory contains
`tokens.json` (separate admin/ingest tokens) and `connection.json`. CLI clients read
these files without creating credentials. Use `--state-dir` consistently.

Responses use `ding.ing/v1alpha1` envelopes with either `data` or a stable
`error.code`. `/health` is public liveness. Every `/v1` route requires a bearer
token; only `/v1/ingest/{watchId}` accepts the ingest token. The admin token cannot
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
