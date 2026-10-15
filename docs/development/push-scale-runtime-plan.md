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
