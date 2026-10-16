# PostgreSQL backend and cloud architecture

This is the original P4/P5/P7 contract in the [performance plan](push-scale-plan.md).
The October 15 [Cloud PostgreSQL plan](cloud-postgres-plan.md) now defines execution
order: efficient single-server Cloud first, then multiple servers. The shared Go
evaluator and acceptance contract below remain authoritative for both databases.

## G1 Establish a backend contract in Go

Inventory `internal/store`, `internal/serverstate`, their migrations, transaction
callbacks, health, backup and all hosted callers. Existing SQL depends on SQLite
JSON functions, PRAGMAs, `total_changes`, temporary tables, integer sequences and
file locks. The workspace and control packages contain about 125 files together;
this is a real adapter project.

Keep domain records and `Store`/`Tx` facades stable where practical. Introduce
backend transaction/query implementations behind them, with explicitly reviewed
SQL per backend. Avoid a global placeholder-rewriting layer or a generic ORM that
moves evaluation into database-specific code. Convert operation families in small
increments: watch/revision records, entities/timers, observations/receipts/usage,
events/evidence/outbox, inspection/retention, then control identity and credentials.

Run a common semantic contract against SQLite and PostgreSQL. Replace SQLite's
audit change detection with explicit mutation accounting on PostgreSQL. Preserve
actor attribution, no-op behavior and transaction boundaries. Health/Doctor must
report backend-specific durability, storage and readiness without pretending
PostgreSQL has SQLite PRAGMAs or one file per workspace.

**Exit:** a SQLite-only facade refactor passes existing tests before introducing
PostgreSQL behavior. Each PostgreSQL operation family passes real-database contract
tests; mocks do not establish transaction or isolation parity.

## G2 Create the isolated PostgreSQL lab

Add an opt-in Compose configuration under `benchmarks/push`, using PostgreSQL 17
with a verified patched release and image digest. The Supabase changelog records
a PostgreSQL 17 update path; verify supported versions and extension compatibility
when choosing the exact image. Use no optional extensions for the initial store.
[Supabase PostgreSQL update](https://supabase.com/changelog/postgres-15-19-17-11-breaking-changes).

Use a dedicated database/volume/network and generated fixture roles. Bind published
ports to loopback. Keep credentials in private fixture files, redact DSNs and record
resource/durability configuration in the manifest. Start only owned test resources;
preserve failed-state evidence before cleaning them up.

Use `pgx/v5`'s `database/sql` adapter with explicit connection limits, acquisition
timeouts, connection lifetime and statement tracing. Start with direct connections
and one application process. Record actual database server version and image,
driver version, pool configuration, schema version and application commit.
[pgx driver and interfaces](https://github.com/jackc/pgx).

**Exit:** fixture creation, schema migration, transaction rollback, reopen and
backup/restore work locally. Starting a database alone is not a Ding throughput
result. A full local Supabase stack and its separately connected local MCP endpoint
can be an optional compatibility target after the adapter works.

## G3 Preserve workspace isolation and receipt sequences

Use private control/runtime schemas in one PostgreSQL database. Every tenant-owned
primary key, foreign key, unique constraint, index and query includes the workspace
identity where it scopes the record. Existing event IDs, watch names, delivery IDs
and sequence numbers can coincide across physically separate SQLite stores.

Preserve workspace-local observation ordering and contiguous batch ranges using a
transactional range allocator. A global PostgreSQL sequence can interleave other
workspaces' rows and advances independently of rollback; it cannot be substituted
for the existing receipt-range contract. Allocate ranges for actual appended
observations, including internal observations where required. Reconcile after
concurrent writes and rollback.

Preserve nanosecond timestamp/expiry behavior with explicit integer storage where
needed. Keep canonical definitions, receipt/provenance bytes and identifier inputs
unchanged where hashing or replay depends on them; choose JSONB only for fields
whose semantics and query needs justify it.

Use transaction-local workspace context and RLS for tenant runtime tables with a
non-owner, non-superuser, non-bypass application role. Migration/backup roles are
separate. Keep schemas outside Supabase's exposed Data API; existing Ding HTTP/Go
authorization governs access. Test context reuse through the pool, missing scope,
cross-workspace foreign keys and identical IDs in unrelated workspaces.

**Exit:** negative isolation tests cover ingest, inspect, event evidence, retry,
credential lifecycle, export and cleanup with two independently enrolled users.
Workspace scoping must be enforced before enabling shared cloud storage.

## G4 Implement atomic acceptance and accounting

Within one PostgreSQL transaction, validate the current grant/key, workspace and
watch lifetime/revision, reserve quota, allocate sequences, evaluate state, append
observations/events/outbox and save usage plus the receipt. Return HTTP 202 only
after the durable commit. Enforce the receipt namespace with a database unique
constraint; simultaneous identical retries resolve to the same saved identity.

Define deterministic lock ordering across authorization, workspace accounting,
watch and entity records. Mutable authorization rows must use a protocol that
conflicts with revocation/scope/deletion writes for the acceptance duration. A Go
process mutex is insufficient across two processes. Sample expiry after waiting
for the relevant locks. Bound deadlock/serialization retries and resolve uncertain
commit outcomes through the same idempotency key.

Keep same-watch evaluation serialized initially. Workspace sequence/usage rows can
also become hot; instrument their lock waits and report that limit. PostgreSQL does
not remove contention created by the acceptance contract. Changes to ordering or
counter partitioning require a separate parity experiment.

Define PostgreSQL workspace storage quotas explicitly: logical retained bytes and
pending counts can be accounted transactionally; shared physical database/index/WAL
size needs a host-level reserve. SQLite's page cap has no direct equivalent in a
shared PostgreSQL database. Compare ordinary performance with nonbinding quotas,
and qualify each backend's quota behavior separately before cloud release.

**Exit:** batch atomicity, exact usage, duplicate/conflict, revoked/expired keys,
lifecycle races, fault rollback and uncertain commits pass through real HTTPS on
both backends. Cloud admission cannot enable until quota semantics are reviewed.

## G5 Coordinate workers across application processes

Replace filesystem/process ownership assumptions with database-backed leases and
fencing tokens for acquisition, timers, retention and delivery. Use database time
for distributed lease expiry. A worker that lost ownership cannot commit stale
evaluation, cancel a new owner's work or acknowledge a reclaimed delivery.

Claim outbox work transactionally. `SKIP LOCKED` can distribute eligible work,
but the eligibility/stream-claim protocol must prevent claiming a later event
while an earlier event in the same ordered stream is leased or awaiting retry.
Perform receiver I/O outside the transaction and update results only with the
current lease token. Independently scoped streams can run concurrently.

**Tests:** two separate Go processes sharing the same database; identical and
different keys at the same time; revoke credentials from one while the other
accepts; terminate a worker mid-poll, mid-timer, mid-maintenance and mid-delivery;
delay lease renewal; reconnect after a database/network interruption. Verify one
durable acceptance, correct sequences and no concurrent ownership of ordered work.
Retain stable IDs for potentially duplicated external notifications.

**Exit:** distributed correctness passes before any horizontal capacity claim.
Compare one versus two processes under the same total CPU/RAM budget, then report
additional-resource scaling separately.

## G6 Evaluate the connection pooler separately

Start with Go's bounded direct SQL pool. Sweep a small connection range with
1/10/100 workspaces and one/two processes, recording acquisition waits, actual
database connections, row-lock waits, throughput, CPU and memory.

Then add a pinned PgBouncer fixture and repeat the selected cases in transaction
pooling mode. Avoid session-dependent settings/locks and preserved temporary
tables. Use transaction-local workspace context; convert retention's SQLite
temporary-table assumptions to safe backend-specific operations. Verify driver
prepared-statement behavior against the chosen pooler settings. Current PgBouncer
supports protocol prepared plans when configured; SQL `PREPARE` and session
features have different restrictions. [PgBouncer feature matrix](https://www.pgbouncer.org/features.html).

**Tests:** reused connections across workspaces, rollback and cancellation,
statement preparation, pool exhaustion, pooler restart, database restart and
connection churn. Resolve uncertain acceptance with the original key.

**Exit:** add the pooler to the proposed deployment only if measured connection
pressure or process count warrants its extra configuration and failure surface.
For a later Supabase comparison, qualify that deployment's actual pooler/driver
combination independently; do not transfer PgBouncer results automatically.

## G7 Select the cloud backend and rehearse migration

Produce a decision record comparing optimized SQLite, direct PostgreSQL and
pooled PostgreSQL. Include workload-specific stable capacity, tail latency,
accepted observations per CPU-second, total app/database/pooler memory, retained
storage, recovery time and expected operating complexity/cost. Select PostgreSQL
when it meets correctness/recovery gates and supplies needed capacity or
availability that justifies the added service. Keep local SQLite independently.

Before cloud cutover:

1. Implement a versioned backend-aware export/import format. Import control and
   workspace records consistently, preserving workspace/watch lifetimes, logical
   producers, receipts, ranges, event IDs, evidence, transfers and encrypted vault
   bindings. Handle sessions and recovery identity under existing restore policy.
2. Rehearse on synthetic populated databases, then on a privately restored copy.
   Restore starts in quarantine with watches paused and old access invalidated;
   validate safe recovery before enabling execution or delivery.
3. Verify a complete PostgreSQL backup/restore, including control metadata,
   runtime state and encryption keys kept separately. Measure recovery time and
   the data-loss window; managed snapshots/PITR require their own tested procedure.
4. Adapt startup, readiness, maintenance and Doctor to explicit backend selection.
   Refuse a failed PostgreSQL configuration rather than silently opening empty
   SQLite state. Qualify production SQL TLS and secret handling.
5. Plan a bounded maintenance cutover with admissions stopped, workers quiesced,
   a consistent encrypted export, import verification and one enabled destination
   backend. Record producer retry expectations and actual downtime. Reconcile all
   acknowledged receipts before accepting new inputs.
6. Test rollback before PostgreSQL accepts new writes. After new acceptance,
   returning to an old SQLite snapshot would lose data: require reverse transfer
   and reconciliation or a forward repair. Avoid an unqualified dual-write phase.

**Exit:** migration and recovery are reviewable and tested. Complete the existing
Cloud soak and its separate cleanup before scheduling a production cutover.
Production adoption is a later deployment decision, not an outcome of this plan.
