# Ding Cloud: efficient PostgreSQL first, multiple servers next

Plan revised October 15, 2026. **SQLite remains the fast local default. Ding Cloud
will target PostgreSQL, initially with one Go server and one database server.**
Qualify that inexpensive topology before adding application replicas. This is the
next implementation plan; it does not describe an already qualified deployment.

## Product and architecture decisions

- Keep both backends in the same Go application. Self-hosters explicitly choose
  PostgreSQL; local SQLite needs no external service. Keep backend SQL/migrations
  separate behind the existing Go store contracts.
- Keep parsing, evaluation, authorization, scheduling, quotas and delivery policy
  in Go. SQL supplies persistence, indexes, atomic operations and concurrency.
- Start Cloud with direct, bounded PostgreSQL connections and durable storage,
  with app and database near each other on a private network. Measure verified
  TLS. Add a pooler, cache service or broker only for a demonstrated requirement.
- Preserve SQLite's per-workspace files, WAL/FULL durability and inexpensive
  in-process execution. PostgreSQL coordination must not become a local dependency.
- One app plus one database is the initial cost/capacity target. It is not high
  availability: app restarts interrupt service, and database failure can require
  recovery. Replicas, failover and their costs require their own qualification.

The current adapter permits only one active application through a global owner
lock. Multi-server coordination and SQLite-to-PostgreSQL migration remain
unfinished. The [October 15 results](push-scale-results.md) establish neither a
production PostgreSQL ceiling nor an expected multiplier from adding servers.

## 1. Measure and remove the single-server bottlenecks

Implement in this order. Each numbered item is a review milestone containing
separate, tiny commits; it is not one large commit.

1. **Make database cost attributable.** Extend `internal/runtimeperf`,
   `internal/database`, and the lab with bounded operation labels for SQL calls,
   transactions, pool wait/churn, worker work and empty checks. Capture PostgreSQL
   statement statistics, locks, WAL/checkpoints and CPU throttling. Trace one
   accepted request and an idle instance before optimizing. Never record payloads,
   credentials or tenant IDs in public metrics. Separate profiling from capacity.
2. **Stop doing database work for every idle workspace every second.** Refactor
   `internal/watchrun/workers.go` and `internal/cloud/pool.go` around persisted
   next-due work, indexed due queries and bounded dispatch. Coalesce schedule
   changes with the transaction that creates work. Wake local workers after commit;
   use periodic durable reconciliation to recover lost wakeups. PostgreSQL may
   use notifications as hints. Scope any cross-workspace dispatch metadata narrowly;
   its claims must still enter the tenant transaction with authorization and RLS.
   SQLite uses its existing local ownership and in-process wakeups. Empty delivery
   queues must not repeatedly open write transactions. Initial engineering target:
   at most five runtime SQL transactions/s per idle Go server with 10, 100 or 1,000
   push-only workspaces, no due timers and empty outboxes. Measure maintenance
   separately. Due-work dispatch p99 must stay within one second at qualified load.
3. **Reduce request round trips without weakening acceptance.** In
   `internal/store/postgres.go`, `internal/database/ownership.go` and
   `internal/serverstate/ingest_guard.go`, combine scope setup, batch compatible
   reads, and consolidate repeated ownership checks where the fencing proof permits.
   The current guarded write has roughly 13 wrapper interactions around domain
   SQL; confirm that with tracing. Preserve deterministic lock order, fresh
   revocation/expiry checks and transactional tenant context. Commit each query
   change with its race/isolation tests and a measured call-count comparison.
4. **Batch writes and accounting.** Reserve contiguous sequence ranges once per
   actual batch; batch observation/event/outbox persistence without changing Go
   evaluation order. Profile per-row JSON conversion and repeated updates to
   `workspaces.logical_bytes` in `internal/store/postgres/006_storage.sql`.
   Aggregate accounting deltas per statement/transaction where safe. Preserve the
   documented logical-byte definition, quota rejection, audit attribution and
   rollback behavior; qualify repair/backfill with randomized reconciliation.
   Keep the workspace ordering lock initially. Narrow it only in a separate change
   if measured contention and a correctness proof justify that complexity.
5. **Tune the direct Go pool and query plans.** Compare 2/4/8/16 query connections,
   idle retention, connection lifetime and prepared-plan caching. The current pool
   keeps only two idle connections and disables the statement cache; their actual
   cost is unmeasured. Set total connections with maintenance/operator headroom.
   Inspect slow plans and add only justified indexes, measuring write/storage cost.
   Keep a separately qualified compatibility mode if a pooler is later introduced.

The evidence behind this order is concrete: a retained 100-workspace sample at
25 pushes/s issued approximately 100 empty delivery claims/s and 382 snapshot
operations/s. A separate window recorded about 647 PostgreSQL transactions/s,
including normal read rollbacks. These are overlapping categories, not additive
costs. They point to avoidable application work; they do not prove that PostgreSQL
itself has reached its capacity.

**Exit:** backend correctness passes, idle work meets its budget, and the
single-server Linux capacity test produces repeatable operating rates. Shared Go
changes must preserve SQLite's measured gains and low idle resource use.

## 2. Qualify a business-ready single-server deployment

- Exercise real enrollment, independent customers, service keys, browser/MCP reads,
  HTTP polling and JSON push together. Include noisy-tenant fairness, finite queues,
  admission limits, cancellation, overload responses and recovery after pressure.
- Keep HTTP 202 after durable receipt, usage, observations, evaluation and outbox
  commitment. Resolve an uncertain response using the original idempotency key.
  Verify tenant isolation, credential revocation and exact accounting on real SQL.
- Expose actionable pool/lock pressure, acceptance latency, oldest due work,
  delivery backlog, physical storage/WAL and retention health. Readiness must fail
  on unusable storage; overload must remain bounded and recover without restart.
- Test app/database process crashes, lost connections, disk pressure and restoration
  on disposable Linux fixtures. Implement automated encrypted backups, separate
  key recovery, restore verification and a tested WAL archive/PITR procedure.
  Proposed initial recovery objectives: restore service within 30 minutes and
  lose at most five minutes after complete database-storage loss. Measure these
  on a stated dataset; label them unmet until demonstrated. A normal durable app
  restart must lose zero acknowledged receipts. PITR may rewind acknowledged work
  within its declared recovery window; report that explicitly.

**Exit:** publish a one-page capacity/cost result and recovery runbook. Keep
PostgreSQL experimental until these gates pass. Qualification work and the active
Cloud soak use separate resources and revisions.

## 3. Finish multi-server support

Keep the efficient single-server path, then implement these independent increments:

1. Replace the global execution owner with node registration plus database-time
   leases and monotonically changing fencing epochs for due work. Retain exclusive
   maintenance/migration ownership that refuses active runners. Bound transactions
   so a stalled owner cannot block takeover indefinitely. Every ownership-dependent
   mutation checks its epoch atomically; stale workers cannot commit after takeover.
2. Coordinate acquisition, timers, retention and delivery separately. Claim only
   eligible ordered-stream heads with transactional leases/`SKIP LOCKED`; run
   receiver I/O outside SQL transactions and finish only with the current token.
   Preserve stable delivery IDs. External delivery remains at least once: a crash
   after receiver success can cause a retry. Record and reconcile those duplicates.
3. Audit process-local workspace caches, enrollment/deletion, sessions, credentials,
   watch revisions and admission controls in `internal/cloud`/`serverstate`.
   Use durable generations and transactional checks for correctness; invalidation
   messages only accelerate refresh. Concurrent ingest on different nodes must
   preserve one receipt, ordered workspace sequences and current authorization.
4. Define which limits are per node and which belong to the whole customer/instance.
   Keep local concurrency caps for resource protection. Enforce shared customer
   request/byte quotas with transactional counters or bounded durable reservations;
   define burst/error bounds and reclaim behavior before implementation. Adding
   nodes must not silently multiply customer quotas or duplicate scheduled checks.
5. Make node bootstrap validate the same instance identity, encryption key and
   compatible configuration/schema. Remove runtime dependence on shared local files;
   keep ephemeral node state local. Validate load-balanced sessions and operations.
   Start with coordinated maintenance upgrades; rolling upgrades need an explicit
   compatible-version protocol. Qualify two nodes before four.

**Exit:** the Linux suite proves duplicate/revocation races, node death, stale-owner
recovery, database/network interruption and ordered delivery. Compare fixed total
resources separately from added hardware. No linear-scaling promise: a hot
workspace's ordering lock or the database can still be the limit.

## 4. Rehearse migration and enable Cloud PostgreSQL

Complete the versioned transfer described in [G7](push-scale-postgres-plan.md#g7-select-the-cloud-backend-and-rehearse-migration).
Preserve control/workspace identities, producers, receipts, ranges, evidence, usage
and vault bindings. Distinguish disaster restore's intentional credential
revocation/quarantine from a planned backend migration's credential continuity.
Test both policies and prevent simultaneous source/destination execution.

Rehearse quiesce, consistent backup/export, import, exact reconciliation, recovery
and controlled reopen on a private restored copy. Verify rollback before new
PostgreSQL writes; afterward require reconciled reverse transfer or forward repair.
Keep one authoritative backend throughout. The production cutover is a separate
reviewed deployment after the existing soak finishes and its evidence is preserved.
Single-server Cloud adoption can follow stages 1–2 plus this migration gate while
stage 3 is completed separately; enabling a second server requires stage 3 to pass.

## Implementation discipline

Commit each schema change, query optimization, worker mechanism or correctness
fix with its directly related tests before starting another independent change.
Keep measurement/report commits separate and preserve the small commits in PRs.
Run both real-backend contracts for shared changes and targeted race tests for
concurrency changes. Never change durability settings to manufacture a speedup.

The [Linux qualification specification](cloud-postgres-linux-tests.md) defines
resource profiles, workload mix, pass conditions, recovery tests and cleanup.
The original [backend contract](push-scale-postgres-plan.md) and
[test invariants](push-scale-test-plan.md) still apply; the old Mac Docker
measurements remain historical evidence.
