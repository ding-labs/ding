# Ding performance and database implementation plan

Proposed October 14, 2026, Pacific time. Improve the measured Go bottlenecks,
build an experimental PostgreSQL backend, and compare both databases through the
same real ingestion service before selecting Ding Cloud's next storage architecture.
The owner deferred the remaining local runs. The independent Cloud soak continues
on its recorded application revision.

## Decisions

1. Keep ingestion, authorization, evaluation, receipts, quotas, retention,
   scheduling and delivery in Go. TypeScript continues to serve client interfaces.
2. Keep SQLite as Ding's local default. Implement improvements that benefit the
   existing service and can carry over to PostgreSQL.
   Offer both backends as an instance configuration choice for open source
   self-hosting and Ding Cloud, using the same Go application and image.
3. Use an isolated PostgreSQL 17 container for the first comparison, with a
   verified patched version and pinned image digest. Aligning the initial major
   with Supabase's documented PostgreSQL 17 path makes a later compatibility run
   easier. Record and reassess the major when implementation starts.
4. Use `pgx/v5` through `database/sql` initially, preserving the current Go
   transaction API while making its implementation portable. Use one bounded Go
   connection pool; measure direct connections before adding PgBouncer.
5. Build PostgreSQL support for both control metadata and workspace state.
   A query microbenchmark or a workspace-only port cannot establish complete
   Ding Cloud throughput.
6. Qualify one application process first, then distributed worker and authorization
   coordination before enabling multiple processes against PostgreSQL.
7. Select a production cloud backend after correctness, sustained performance,
   recovery and operating-cost evidence. The local experiment creates no cloud
   database or deployment change.

Supabase is a useful later managed PostgreSQL option. Its CLI can run a local
stack, and its local MCP endpoint is available through that stack. The connected
Supabase tools supplied documentation successfully; using a local MCP requires
connecting the client to the local endpoint. Direct Go SQL remains the measured
application path. [Supabase local development](https://supabase.com/docs/guides/local-development),
[Supabase MCP](https://supabase.com/docs/guides/ai-tools/mcp),
[pgx driver](https://github.com/jackc/pgx).

## Evidence and limits

The [preserved baseline](push-scale-baseline.md) includes 88 completed cases and
one cancelled case. Three-minute controls already expose unstable latency at
250–500 requests/s into one workspace and inconsistent results at 1,000/s across
ten workspaces. Profiles locate shared credential connection waits, workspace
transaction serialization, and full group-state loading. Repeated receipt counts
grow with history. Slow delivery accumulates backlog independently of acceptance.

These findings justify focused changes. They do not predict a PostgreSQL speed
multiplier. SQLite permits one WAL writer per database; Ding has separate
workspace databases plus a shared control database. PostgreSQL's concurrency
still depends on the application's transactions and hot rows.
[SQLite WAL](https://www.sqlite.org/wal.html),
[PostgreSQL locking](https://www.postgresql.org/docs/current/explicit-locking.html).

## Implementation order

| Phase | Deliverable | Exit condition |
| --- | --- | --- |
| P0 | Freeze evidence and repair measurement qualifications | Completed, cancelled, profiled and sleep-affected cases are distinguishable; focused controls reproduce on an awake host |
| P1 | Remove unnecessary state and receipt work | Routing and receipt parity pass; affected boundary cases show repeatable improvement |
| P2 | Improve control and workspace read concurrency | Revocation, lifecycle, close/backup and snapshot races pass; shared-workspace and many-workspace latency improves |
| P3 | Bound delivery and maintenance work | Delivery ordering/recovery and evidence retention pass; backlog and maintenance stalls are measured explicitly |
| P4 | Introduce the Go store boundary and PostgreSQL implementation | Backend contract suite and real hosted ingest pass with both stores; workspace isolation and quota semantics are explicit |
| P5 | Qualify PostgreSQL coordination and pooling | Two processes preserve authorization, ordering, receipts and worker ownership; direct versus pooled results are recorded |
| P6 | Rerun the complete matrix and sustained cases | All results reconcile; capacity is reported by workload and total resource budget |
| P7 | Choose cloud architecture and rehearse migration | Recovery, cutover and rollback are tested before a reviewed production deployment |

Keep P1 changes independent: targeted group reads first, receipt accounting next.
Separate control-database pooling from workspace read concurrency in P2.
Build PostgreSQL behind an explicit experimental configuration after those
low-risk improvements have a measured SQLite control.

The [Go runtime implementation](push-scale-runtime-plan.md) specifies the source
boundaries, correctness tests and focused measurements for these changes.
The [PostgreSQL implementation](push-scale-postgres-plan.md) defines the adapter,
tenant schema and transaction guarantees for the database experiment.
The [test and comparison plan](push-scale-test-plan.md) defines measurement
qualification, correctness gates and total-resource comparisons.
The [configurable storage design](push-scale-storage-plan.md) defines separate
backend SQL/migrations, runtime configuration and open source self-hosting.

## Behavior that every increment preserves

- HTTP 202 follows durable commitment of observations, evaluated state, events,
  outbox intents, usage and receipt together. A failed batch leaves none behind.
- Identical bytes/key preserve receipt identity; changed bytes conflict. Credential
  rotation, compatible revision changes and evaluator resets preserve the existing
  logical producer/watch-lifetime namespace.
- Revocation, expiry, workspace deletion and watch lifecycle changes fence
  acceptance. A stale cached credential cannot bypass the commit boundary.
- Accepted time and committed sequence order govern evaluation. Group isolation,
  source-health fanout, missing-data timers and rolling windows retain semantics.
- Existing account/session identity, encrypted secret binding, transfer and restore
  behavior remain part of the backend contract. PostgreSQL needs no Auth replacement.
- Every queue, pool, cache, retry and retention batch has a finite bound and visible
  pressure. Durable input and open-incident evidence remain recoverable.

## Git and review sequence

Use small PRs by phase, keeping surgical commits within each. A coherent change
and its directly related verification are committed before the next independent
change. Keep documentation, schema backfill, runtime behavior and benchmark
evidence in separate increments when they can be reviewed independently.
Preserve commit history when pushing; no squash.

Each performance PR includes its source revision, focused before/after results,
correctness gates and a clear rollback. Merge verified increments to main without
redeploying the application used by the active Cloud soak. Once implementation is
authorized, begin with P0/P1; this document is the implementation plan, not a claim
that optimizations or a PostgreSQL port have landed.
