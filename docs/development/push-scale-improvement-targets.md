# Push performance improvement targets

These targets come from local stress tests of application commit
`f8cd4f8b8d55f792f2e3674c68100acec1eea137` on October 14, 2026. They are
measurement hypotheses for follow-up changes, not implemented optimizations or
Railway capacity claims. The cloud soak continues independently.

| Target | Evidence | Next experiment |
| --- | --- | --- |
| Shared credential reads | The control database has `SetMaxOpenConns(1)`. A 30-second, 100-workspace, 1-CPU screen attributed about 67% of aggregate blocking time to `ServiceToken` waiting for a connection. A three-minute native ten-workspace run attributed about 85%. | Compare bounded concurrent credential reads and fewer duplicate lookups. Preserve the authorization fence and prove revocation behavior. |
| Workspace serialization | `Store.Update` and `Store.View` hold the same Go mutex for their entire transaction. A three-minute native single-workspace overload run attributed about 93% of sampled mutex wait to `Store.Update`. | Separate safe snapshot reads from serialized writes, shorten callbacks, and test ordering, lifecycle changes and replay races. |
| Full group-state loading | `acceptPush` calls `tx.Entities(id)` and builds every existing key for each observation. In a 1-CPU screen with all 10,000 groups seeded, this consumed about 69% of CPU samples; acceptance p99 was 793 ms at 25 requests/second. | Read keys/state only as routing requires. Preserve source-health fanout, missing-data behavior and group limits. |
| Active receipt counting | Every new acceptance executes `count(*) WHERE expires_at > ?`. The cost grows with retained receipts. It consumed about 19% of CPU samples in one constrained overload screen, but only about 5% in a longer native run. | Compare exact transactional counters and bounded expiry cleanup against the current indexed count; repeat aged-store tests. |
| Notification backlog | A 1-CPU, 30-second screen at 25 requests/second left 229 deliveries after a one-minute drain with a 100 ms receiver. | Measure dispatch scheduling and ordering constraints separately from ingestion. Bound retries/backlog and preserve event identity. The test receiver shares server CPU and syncs its ledger, so this is not an external delivery capacity measurement. |
| Retained history | Three-minute native single-observation runs added about 1.5 KB to the main database per accepted input. | Measure live pages, database and WAL separately; compare payload projection, batches, receipt retention and history cleanup. Avoid extrapolating short snapshots into a retention guarantee. |

Profile percentages refer to different experiments and denominators. CPU samples,
aggregate goroutine blocking time and mutex wait cannot be added together or
converted into a predicted throughput gain. Queueing profiles identify where work
waits, not necessarily the sole cause of the delay.

## What PostgreSQL would change

PostgreSQL would provide a central store usable by several application instances
and concurrent transactions with snapshot reads. SQLite WAL supports simultaneous
readers and a writer, but permits only one writer per database and requires its
shared files to remain on one host. Ding has a separate workspace database for
each tenant, plus a shared control database; the writer restriction is not a
single writer across all workspace stores.

A PostgreSQL migration would still need to remove the Go transaction mutex and
one-connection settings to benefit from that concurrency. Full state loads,
repeated receipt counts, hot usage/watch/entity rows and notification ordering
would remain engineering work. A shared cloud schema also needs workspace-bound
keys and queries; current store queries rely on physical workspace isolation.
Multiple app instances require distributed worker ownership, claims and
authorization fencing before they can execute safely.

Go already has a database connection pool. PgBouncer can share a bounded set of
PostgreSQL connections among clients; it does not remove expensive queries or add
database CPU/storage throughput. Transaction pooling requires compatible session
behavior, and statement pooling cannot support Ding's multi-statement acceptance
transactions.

The useful comparison is optimized SQLite against a cloud PostgreSQL backend on
the same workloads and durability requirements, counting both application and
database CPU/RAM. No PostgreSQL benchmark has been run, so no speed multiplier or
percentage of issues solved is established. A cloud PostgreSQL backend can coexist
with local SQLite while the business logic stays in Go.

Sources: [SQLite WAL](https://www.sqlite.org/wal.html),
[PostgreSQL concurrency](https://www.postgresql.org/docs/current/mvcc-intro.html),
[PgBouncer pooling modes](https://www.pgbouncer.org/features.html).
