# Daemon integration API

Responses retain `apiVersion: ding.ing/v1alpha1`; capabilities reports the
separate integration contract `ding.integration/v1`. Integration routes live
under `/v1/integrations/`. They accept a paired `Bearer ding_mcp_…` token and
reject browser Origin headers/cookies. General admin routes do not accept
integration tokens; integration tools cannot mint browser sessions or backups.

Administrator bearer credentials are accepted only for these pairing routes:

| Method and path | Body/result |
| --- | --- |
| `POST grants` | `{name, scopes, days, commandRevisions?, secretRefs?}` → `{grant, token}`; plaintext token returned once |
| `GET grants` | Bounded grant metadata; no token hashes or plaintext tokens |
| `DELETE grants/{id}` | Immediate revocation; retained receipts remain inaccessible |

Scopes are `inspect` (required), `preview`, `manage`, `retry`. They are
instance-wide. Up to 100 exact command revisions and 100 environment-reference
names may be allowed locally. Authorization is checked again inside every write
transaction, preventing a revocation race between admission and commit.

| Scoped route | Scope | Behavior |
| --- | --- | --- |
| `GET capabilities` | inspect | Instance identity, permissions, version and bounds |
| `GET watches`, `events`, `deliveries`, `destinations` | inspect | Console bounded read models and opaque cursors; limit 1–100 |
| `GET watches/{id}`, `events/{id}`, `deliveries/{id}` | inspect | Bounded inspection, recorded evidence, delivery attempts |
| `POST preview` | preview | `{manifest, fixture?, watch?}` → diagnostics or `{valid, preview:{handle, expiresAt, changes}, descriptions, fixture}` |
| `POST apply` | manage | `{handle, operationKey}` → committed apply result |
| `POST watches/{id}/lifecycle` | manage | `{action, expected, expectedGeneration, operationKey, cancelPending?}` → saved watch |
| `POST deliveries/{id}/retry` | retry | `{operationKey}` → `{id,status:"pending"}` |
| `GET operations/{key}` | inspect + original mutation scope | `{key,action,result,createdAt}`; 404 means no committed receipt visible at lookup |

Fixture summaries include observation/event counts and the first 20 events, with
`truncated` indicating more. The full CLI fixture report remains available.
Requests allow a 1 MiB manifest and 8 MiB fixture, with two concurrent previews.
The Python adapter caps responses at 16 MiB, refuses redirects, and never
automatically retries a write.

Keys are 16–128 ASCII letters, digits, underscores, or hyphens. Generate a UUID
once per intended action. The key namespace spans all actions for one grant in
one instance. A digest binds immutable arguments; apply binds the preview handle,
which binds the exact manifest and captured preconditions. Receipts and effects
commit atomically. Errors roll back effects.

Errors include `integration_denied` (403), `integration_limit` (429),
`operation_conflict`/`revision_conflict` (409), `preview_expired` (410),
`cursor_expired` (410), and `not_found` (404). The Python adapter reports
`outcome_unknown` after an interrupted write. Reconcile/retry with the same key;
never use a new key to bypass uncertainty.

Schema 4 adds grant, preview, and operation tables. Existing migration safeguards
back up older databases before changing schema. See [retention](privacy.md) for
bounds and receipt lifetime.
