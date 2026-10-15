# PostgreSQL backend and cloud architecture

This is P4/P5/P7 of the [performance plan](push-scale-plan.md). Build and test an
experimental backend locally before choosing a production migration. The shared
Go evaluator and acceptance contract remain authoritative for both databases.

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
