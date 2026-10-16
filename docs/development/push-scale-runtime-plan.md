# Go runtime performance implementation

This is P1–P3 of the [performance plan](push-scale-plan.md). Each numbered change
is an independent implementation and measurement increment. Preserve real
authorization, synchronous durable acceptance and existing condition semantics.

## R1 Read only the group state required by an input

**Source:** `internal/watchrun/app.go` acceptance, `internal/condition/routing.go`,
and entity reads in `internal/store`.

The healthy grouped-input path can derive its canonical entity key without
loading every existing state blob. Separate routing decisions from state loading:

1. Preserve typed match/group-key behavior and validate the complete batch.
2. Derive the distinct keys needed by healthy observations; fetch their states in
   bounded queries inside the acceptance transaction. Read the source-bootstrap
   sentinel explicitly when recovery handling requires it.
3. Reuse loaded and newly evaluated states for subsequent observations in the
   same batch. Evaluate repeated keys in input order.
4. Use an indexed entity count or exact transactional counter for new-group
   admission, including groups first created earlier in the same batch.
5. Preserve full fanout for source-health inputs and clock transitions. Timer
   inputs target their existing entity. Document these intentionally broader paths.

**Tests:** compare observation ranges, entity state, events, evidence and timers
with the original evaluator for matching/nonmatching inputs, typed keys, absent
groups, duplicate groups in a batch, entity-limit boundaries, initial source
health, recovery, unknown values and rollback halfway through evaluation. Run
1/100/1,000/10,000 actual seeded groups with one and 100 outputs per request.

**Exit:** healthy single-group input reads scale with the touched groups. Three
180-second 10,000-group controls improve latency/CPU without changing replay or
state. Land the store read API and its tests before the runtime switch if needed.

## R2 Remove repeated full active-receipt counts

**Source:** `internal/store/push.go`, receipt expiry in `internal/store/lifecycle.go`,
and `acceptPush` admission in `internal/watchrun/app.go`.

Prototype exact active-count metadata with an expiry watermark/next-expiry index.
Receipt insertion, expired-key replacement and expiry adjustment update metadata
transactionally with acceptance. Replays and conflicts never increase it. Use
indexed due-expiry deltas when the watermark advances; schedule cleanup in bounded
batches. Retention must not decrement an already-accounted expiry twice.

Backfill and validate metadata in a versioned migration. A backward clock jump,
missing metadata or detected mismatch triggers an authoritative recount/repair
under the write transaction. Preserve exact admission at the receipt limit;
do not admit from an eventually consistent estimate. Benchmark the prototype
against the indexed count before committing to additional metadata complexity.

**Tests:** insertion, concurrent duplicate keys, changed bytes, expired-key reuse,
mixed TTLs, expiry boundaries, long downtime, forward/backward clock changes,
cleanup interleaved with acceptance, failed transactions, restart and migration
from populated stores. Compare the counter with an authoritative query after
randomized operation sequences.

**Exit:** repeat 0/1,000/10,000/100,000-receipt query tests and real aged-store
ingestion, with actual starting counts verified. The common admission path avoids
a scan proportional to all active receipts while every counter agrees exactly.

## R3 Improve shared credential reads

**Source:** `internal/serverstate/state.go`, `services.go`, service mutation files,
and the authorization fences in `internal/cloud/push.go`.

Introduce a bounded read-only control-database pool alongside serialized SQLite
writes. Route appropriate credential reads to it. Measure pool sizes 1/2/4/8,
pool waits and memory across 1/10/100 workspaces before selecting a default.
Keep `NoteServiceUse` coalesced as diagnostic metadata.

Inventory initial authorization, replay authorization and pre-commit checks.
Remove duplicated decoding or query work only when the same freshness guarantee
can be proved. Keep a fresh authorization check inside the existing revocation
fence. Start without an authorization cache; a future cache needs bounded memory,
expiry, rotation/deletion invalidation and an authoritative commit check.

**Tests:** revoke/update scopes, rotate keys, expire a key while a request waits,
delete a workspace, change watch lifetimes and lose a response concurrently with
ingestion. Use barriers to place mutations on each side of acceptance. Verify
read connections cannot mutate, transactions cannot leak and close drains users.

**Exit:** repeat many-workspace and noisy-workspace cases; compare pool wait and
commit latency while every revocation/lifecycle fence still passes.

## R4 Allow safe workspace snapshot reads alongside the writer

**Source:** `internal/store/store.go`, health/backup/restore, Console inspection,
and the hosted tenant lifecycle.

Keep one SQLite writer per workspace. Separate its ordering gate from the store
lifetime gate, and serve `View` through bounded read-only snapshot transactions.
Shorten write callbacks by preparing immutable parsing/routing work beforehand;
retain all mutable checks and evaluation effects in the write transaction.

Close, backup, migration, page-limit changes and restore must quiesce the right
operations. Bound reader duration and expose WAL/checkpoint pressure. Configure
connections per active workspace together with a host-wide budget so 100
workspaces do not multiply a large pool/cache allocation. Measure idle eviction
and connection-open costs. A higher `SetMaxOpenConns` alone is insufficient.

**Tests:** a deliberately slow read overlapping a write; consistent multi-table
snapshot reads; no dirty reads; context cancellation while waiting; concurrent
close/backup/limit changes; receipt replay against active writes; read/write
stress with the race detector. Confirm read-only enforcement in the actual driver.

**Exit:** real authenticated inspection reads stay responsive under write load;
writer order, WAL/FULL durability, backup consistency and memory bounds hold.
Commit the lifecycle refactor separately from enabling concurrent reads.

## R5 Improve delivery fairness and recovery

**Source:** `internal/watchrun` delivery workers and `internal/store/outbox.go`.

Measure enqueue-to-first-attempt, attempt duration, oldest pending age, retry
delay and drain rate separately from ingestion. Schedule bounded work fairly
across independent watch/destination streams. Keep receiver I/O outside store
transactions, preserve lease fencing and retain per-stream ordering.

A stream with serial 100 ms receiver calls has an ideal ceiling of 10 calls/s
before other costs. The existing 25/s single-stream test intentionally exceeds
that ceiling. More workers or PostgreSQL cannot make its backlog disappear while
that ordering contract remains. Add multiple-stream tests to measure useful
parallelism. Batching notifications or changing ordering is a separate product
decision supported by measurements.

**Tests:** slow and fast streams together, an outage and recovery, retries with
stable event identity, lease expiry/reclaim, worker death during a request,
generation changes and cancellation. Use a separate local receiver process for
delivery capacity; retain the cohosted receiver case as a distinct stress scenario.
Verify durable delivered records exist in the receiver ledger. Delivery remains
at least once, including an uncertain remote acknowledgement.

**Exit:** independent slow streams do not starve fast ones; backlog has an explicit
cause and recovery estimate; restart and cancellation preserve ordering and IDs.

## R6 Bound retention and maintenance transactions

**Source:** `internal/watchrun/maintenance.go`, retention queries in
`internal/store/lifecycle.go`, entity evidence and outbox references.

Instrument maintenance time, rows visited/deleted and writer wait. Move large
expiry/deletion passes to indexed, resumable batches with time/row budgets.
Keep pins for open incidents, rolling windows, missing-data state and pending
deliveries correct while ingestion advances.

The current retention pass computes pins and deletes within one transaction.
Splitting it requires a proven reference protocol: persistent transactional pins
or a versioned revalidation before deletion. Do not release the lock between
pages and then delete against an obsolete snapshot. Prototype that protocol with
small fixtures before changing production retention.

**Tests:** ingestion and delivery while retention runs; a crash between batches;
windows spanning the cutoff; open incidents and source-health evidence; state
changes that add/remove pins; deleted watches; expired receipts with R2 counters;
restart resumption. Compare retained evidence with the existing implementation.

**Exit:** cleanup has bounded stalls, reaches completion and preserves every live
reference. Memory and WAL remain bounded during aged-store churn.

## R7 Reduce storage growth with measured schema changes

Measure logical input bytes, projected observation bytes, receipt/index storage,
live pages, allocated database bytes, WAL and backup overhead separately. Compare
one/18/100 outputs, retained versus discarded padding, and steady retention churn.
Large incoming JSON need not become equally large retained state.

Inspect index use and duplicate metadata before changing encoding or indexes.
Prefer removal of proven redundant work; require a versioned migration, populated
store upgrade test and restore test for each schema change. Benchmark compact
receipt encoding or integer/internal keys only if they materially reduce the
measured dominant storage cost while keeping external identifiers stable.

**Exit:** reduced bytes per accepted observation with equivalent query/replay
behavior. Keep live usage separate from filesystem reserve; SQLite freelist reuse,
WAL and backups all matter to a small production volume.

## R8 Bound inspection and instrument remaining costs

Keep inspection APIs paginated and bounded. Measure read query plans under large
entity/history/outbox populations before adding derived summaries or counters;
update any new summaries in the same transaction as their source data.

Add bounded histograms/counters for auth pool wait, workspace writer wait, parse,
evaluation, commit, maintenance and delivery. Label by finite operation/status
classes; keep tokens, request keys and unbounded workspace/entity IDs out of
metric labels. Expose configured and active connection/worker/queue counts.

After R1–R4, collect profiles in separate diagnostic runs. If JSON allocations or
condition compilation become dominant, prototype bounded immutable compiled-plan
caches and reusable buffers, with revision invalidation, race and memory tests.
Keep parser depth/node/output limits intact. Evaluate write microbatching only if
durable commit cost remains dominant; its receipt, rollback, ordering and latency
contract needs its own design.

**Exit:** new optimization work follows the remaining profile evidence. Repeated
reads preserve API behavior and do not exhaust CPU, memory or connections.
