# Cloud push operations

This runbook accompanies the [implementation plan](cloud-push-plan.md) and [qualification record](cloud-push-qualification.md). The initial topology is one process and one durable volume. Multiple writers or replicas sharing this directory are unsupported. The OS writer lock rejects them.

## Deployment and configuration

Railway now tracks `ding-labs/ding:main`. PR branches target `main`; merging changes triggers a production deployment. The existing Railway image, domain, AuthKit configuration, volume, and readiness contract are retained. Push implementation is not live until its PR is merged/deployed.

Before deploying a schema-changing release, enter the documented [Railway maintenance and backup workflow](../../deploy/railway/README.md), take an encrypted off-volume backup, and verify recovery keys. Control metadata moves from schema 100 to 102, and each workspace moves to schema 7. Both stores save a private pre-migration SQLite snapshot. Each database migration is atomic; all workspace migrations finish before workers start or readiness succeeds. If interrupted, startup resumes using each database's committed schema version. Do not run the old binary against migrated data.

Set ordinary JSON config fields `capacity` and `push`. Railway accepts the same objects as `DING_CAPACITY_JSON` and `DING_PUSH_JSON`; unknown fields, invalid bounds, or trailing JSON prevent startup. These variables contain policy, never producer secrets. Zero values select defaults except cumulative outbound budgets, where zero means no monthly quota.

```json
{
  "push": {
    "disabled": false,
    "nativeWebhooks": false,
    "hostConcurrent": 16,
    "workspaceConcurrent": 2,
    "workspaceRate": 20,
    "workspaceBurst": 40,
    "producerRate": 10,
    "producerBurst": 20,
    "bytesPerSecond": 1048576,
    "byteBurst": 2097152,
    "readSeconds": 5,
    "maxDepth": 32,
    "maxNodes": 32000,
    "maxReceipts": 100000,
    "receiptSeconds": 86400,
    "minFreeBytes": 67108864
  },
  "capacity": {
    "apiRate": 20,
    "apiBurst": 100,
    "maxWatches": 1000,
    "maxDestinations": 100,
    "maxPending": 10000,
    "maxStoreBytes": 134217728,
    "retentionDays": 7,
    "minPollSeconds": 1,
    "maxBodyBytes": 1048576,
    "maxEntities": 1000,
    "maxSamples": 10000,
    "maxOutputs": 100,
    "outboundChecks": 0,
    "outboundDeliveries": 0,
    "outboundBytes": 0
  }
}
```

`push.workspaces` optionally lists a canary set of immutable workspace IDs. Empty means every workspace. `disabled:true` stops new unique inputs while authenticated receipt reconciliation remains available. Definitions, timers, pending delivery, and polling continue; a missing-data watch may fire during an intake outage. Pause a watch separately if that is the intended monitoring decision.

## Capacity boundaries

These are implementation bounds, not measured throughput promises:

- 100 active workspaces per process, matching the encrypted backup inventory limit. AuthKit enrollment has its own explicit ceiling; raise it only within this inventory bound. Distinct users enroll into isolated workspaces through the existing bounded AuthKit path; a member invitation shares the inviter's workspace and is not a tenant-isolation test.
- 100 producers and 100 native connections per workspace; at most two live keys per producer. A producer selects at most 100 watches. Independent producers share the workspace rate/byte/concurrency budget. Rotation does not reset it.
- Incoming body default 1 MiB, and each watch may declare a smaller `limits.maxBytes`. The compiler caps outputs at 100. Preview manifest/fixture fields each remain 64 KiB, with a 1 MiB escaped request envelope. Capability discovery distinguishes preview and ingestion bounds.
- Cloud HTTP acquisition and outbound bodies retain their separate 64 KiB transport bounds and host/SSRF/concurrency controls. Optional cumulative outbound checks, delivery attempts, and byte budgets are configurable; zeros disable monthly quotas. Accepted inbound usage remains durable regardless of optional outbound quotas.
- Workspace allocation is capped at 128 MiB, within the existing 256 MiB backup file envelope. WAL, migration snapshots, control data, backup staging, and filesystem overhead consume additional volume space. One 1 GiB volume cannot accommodate 100 workspaces at their individual maxima. Observe actual use and expand the volume before increasing workload.
- A host disk reserve (default 64 MiB) closes new acceptance before exhaustion; receipt lookups precede this check. This is an admission margin, not a substitute for disk alerts or sufficient backup staging space.
- Generic intake has separate host slots and a global request limiter. Native signature bodies have four host slots and one per workspace before validation. Authenticated invalid and duplicate requests consume admission. Unknown chunked bodies reserve the maximum body byte cost.
- Limiter state resets on process restart; accepted usage does not. Registry entries are bounded and cannot be evicted while requests/rate debt remain. Do not promise a strict durable rate quota.

## Authorization and storage lock order

Read the bounded body and project JSON without `DB.Access`. Resolve trusted workspace/producer identity, then hold `Access.RLock` for final credential revalidation and the short workspace acceptance transaction. Membership, producer/grant/key, and connection changes take `Access.Lock` and revalidate the actor in their control transaction. No control transaction waits on workspace writes. Tenant cancellation ends in-flight ingestion when deletion/shutdown starts. Completed revocation fences later commits; a transaction that committed before revocation may already have succeeded.

Last-use metadata is coalesced and diagnostic. If its write fails after acceptance, the successful receipt remains valid. Accepted usage, projected observations, condition state, event/outbox records, and the receipt commit atomically in the workspace database.

## Observe and diagnose

The operator metrics listener remains loopback-only (Railway `127.0.0.1:8788`). Public `/readyz` and `/healthz` disclose readiness, not metrics or workspace data.

Watch `ding_cloud_ingest_responses_total`, rejection categories, replays, active intake, request and parse/commit latency histograms, heap/runtime allocation, available disk, failed workers, scheduler lag, pending count, and oldest pending delivery. Labels contain only fixed status classes, failure categories, phase, and bucket boundaries. No workspace/watch/repository/key/payload labels are exported. Use the existing Doctor storage health for per-workspace SQLite/WAL/integrity and pinned-evidence pressure.

In the authenticated Console, distinguish:

1. **No last acceptance:** sender URL/key/scope, media type, JSON projection, or intake admission may be wrong.
2. **Accepted input, no condition event:** inspect projected evidence, current grouping/state, and unknown/onUnknown policy.
3. **Condition event, pending/failed delivery:** inspect destination credentials, response status, retry schedule, and outbox age.

Recent request outcomes are a bounded in-memory diagnostic view. A restart clears them. Durable last acceptance, usage, evidence, receipts within their horizon, and outbox survive. `X-Ding-Request-ID` links a producer response to that view. Do not log full HTTP headers or payload bodies to extend it.

## Incident procedures

| Incident | Action and verification |
| --- | --- |
| Compromised producer | Revoke its logical producer or affected key. Confirm old token returns 401 and intake usage stops increasing. Create a replacement with explicit watch scopes. Check authorized events, not payload logs. |
| Routine rotation | Issue a new key with bounded overlap; update producer secrets; retry a known request using the new key and confirm the same receipt. Revoke the old key after producer cutover. |
| Expired/lost key | Inspect key metadata. Rotate, distribute privately, then reconcile original request files within receipt retention. Creating an unrelated producer starts another receipt namespace. |
| Flood / sustained throttle | Identify the authenticated workspace through its diagnostics. Revoke the affected producer or close the canary's admission. Preserve health/admin access; inspect unrelated workspace success and pending delivery age. Do not repeatedly rotate to bypass admission. |
| Full disk / reserve breach | Close new admission; free obsolete exported backups/migration snapshots only after confirming a verified external copy. Preserve current DB/WAL files. Expand storage and check integrity, then retry the same pending request. Never delete receipts/outbox as emergency cleanup. |
| Stalled delivery | Keep accepted input and stable event IDs. Repair destination credentials/network or honor receiver throttling. Expect at-least-once remote delivery. Monitor oldest pending age before reopening a large intake backlog. |
| Failed migration | Keep maintenance active, preserve all databases and pre-migration snapshots, inspect the failing workspace. Restart the same compatible build after fixing disk/access problems. An earlier schema cannot open a newer store. |
| Process restart | Wait for all migrations/workers and readiness. Retry uncertain input with the same key/body; reconcile receipt IDs and usage. An ordinary process crash is different from recovery from an old backup. |
| Snapshot restore | Restore to a fresh directory, retain quarantine, verify ownership/deletions, and release explicitly. Restored sessions, service keys, and native connections are revoked; watches stay paused. Reissue producer grants and explicitly reconcile the snapshot gap before resuming senders. |
| Rollback | Stop intake/execution and take a fresh backup. Prefer a forward fix. To return to a pre-schema binary, restore its coordinated pre-upgrade control/workspace backup into a fresh quarantined directory; acknowledged inputs after that snapshot require reconciliation and may be lost. Never copy one old workspace DB into a newer live control set. |

The deployment owner must configure external alerts and an incident contact for sustained 5xx, free-disk reserve, growing outbox age, failed workers, and expired canary credentials before public availability. This repository does not silently create monitoring subscriptions or send alerts to a new destination.

## Qualification driver

Create a private producer file (token environment-variable names, no inline tokens):

```json
[{"url":"https://cloud.ding.ing/v1/ingest/ci-results","tokenEnv":"DING_CANARY_A","body":{"repo":"canary","workflow":"push","runId":"canary","failed":false}}]
```

```sh
ding-cloud-bench --push-producers producers.json --ledger /private/new-ledger.jsonl --rate 10 --duration 10m
```

The driver creates a new private ledger and fsyncs the offered key/body before sending. It writes each attempt's status and receipt afterward, uses HTTPS verification, refuses redirects, and retries identical bytes/keys. It reports offered, accepted, duplicate reconciliations, rejected, unconfirmed, attempts, generator pressure, and end-to-end acceptance p95/p99. Sixteen workers and a bounded queue prevent unlimited driver concurrency. There is no automatic restart/replay of the ledger; reconcile unconfirmed entries explicitly within receipt retention. Keep synthetic bodies free of personal data because the producer-side ledger stores them.

Run short quiet, noisy-neighbor, burst, slow-body, destination-outage, restart, and backup/restore cases before the separate 72-hour external soak. Reconcile successful receipts with workspace counters and a durable destination receiver ledger. A laptop-local benchmark does not qualify Railway capacity or external notification reliability.
