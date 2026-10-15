# Push performance and database results

October 15, 2026. Local qualification is in progress. The tables below distinguish
completed measurements from the sustained runs still awaiting reconciliation.
No cloud application deployment or database change is part of this experiment.

## What was implemented

SQLite remains the default. The shared Go server can select PostgreSQL explicitly
for a fresh self-hosted installation; see [database operations](../operate/databases.md).
Both backends use the same Go authorization, evaluation, receipt, quota, retention
and delivery logic, with separate SQL and migrations. Configuration does not
silently fall back to SQLite if PostgreSQL is unavailable.

The runtime changes fetch only touched group state on healthy input, account for
active receipts transactionally, separate credential and workspace snapshot
reads from serialized writes, schedule independent delivery streams fairly and
bound retention batches with persistent evidence pins. Source-health transitions
retain their broader group fanout. Timing and pool metrics use bounded labels.
No speculative compression or condition-plan cache was introduced.

PostgreSQL has a bounded query pool and one additional direct ownership session,
forced tenant row security, tenant constraints and transactional authorization
fences. A second server cannot operate concurrently against the same database.
Native encrypted backup and quarantined restore are implemented. Cross-backend
data transfer, transaction poolers and horizontal operation need separate work.

## Reproducible method

The [harness instructions](../../benchmarks/push/README.md) describe how to repeat
individual cases. The full producer journal, raw profiles, private fixtures and
database snapshots stay outside the repository. The published aggregate contains
sanitized measurements and relative evidence identifiers.

| Setting | Measured configuration |
| --- | --- |
| Host | Apple M3, 8 CPU cores, 16 GiB memory; Docker Desktop Linux arm64, 8 CPU and approximately 7.65 GiB available |
| Default service budget | 1 CPU and 1 GiB total for application plus database |
| SQLite allocation | Entire service budget to Go application and embedded SQLite |
| PostgreSQL allocation | Half the service CPU and memory to Go; half to PostgreSQL 17.11, unless the row explicitly specifies a CPU-share sweep |
| Expanded budgets | Explicitly labelled 2 or 4 CPU and GiB total; same default PostgreSQL split |
| Producer | Separate Linux container, 2 CPU / 512 MiB; Linux loopback TLS, outside service budget |
| Delivery receiver | Separate 0.5 CPU / 256 MiB container for delivery capacity cases |
| Durability | Real authenticated HTTP, durable acceptance, SQLite WAL/FULL or PostgreSQL durable commits; no producer retries |
| Workload limits | Most lab cases raise finite admission limits, use a 4 GiB store envelope and one million receipts; deployed defaults have a separate case |
| Arrival and latency | Fixed planned arrival schedule; acceptance latency includes queue and producer journal delay |
| Measurement | Profiles off; 30-second screens, three 180-second trials at selected boundaries, then selected 1,800-second cases |
| Inspection | One aggregate sequential 100 ms ticker, targeting 10 authenticated reads/s across the configured workspaces; actual completed reads are reported |
| Recovery check | Graceful worker/store close and reopen; this does not simulate power loss |

The candidate executable used for final qualification is from
`6f7b3b4f2387a1d7b791d2bc54956514cf3b57de`; its SHA-256 and full source revision are recorded per run. The matched
SQLite control executable is from `95737a02d2a45a249493ac14ac3dc32034001553`.
Earlier screens retain their own source revisions. Later controller changes make
any HTTP 429 disqualify capacity, including rows originally reported stable.
Host-forwarded historical runs and Linux-loopback results are separate families.

A qualified point needs at least 99.9% first-attempt acknowledgements, zero
unexpected rejections (including throttles), uncertain responses, missed arrivals,
read errors or clock discontinuities; acceptance p99 below 250 ms; and successful
receipt, sequence, identity, usage, reopen and postcheck reconciliation. Delivery
cases additionally need their backlog drained and receiver evidence reconciled.
An overloaded point can preserve every acknowledged input and still fail capacity.
Missing observer/final report evidence is inconclusive, even if an offline audit
later finds every acknowledged receipt.

The host is shared. Docker's earlier engine interruption and host-forwarding
failure are preserved separately. At 13:30:06 UTC, the owner authorized restoration
of another project's stopped container during the first SQLite sustained run.
Both that container and its independent retry used about 2% CPU at the initial
sample. Container quotas do not eliminate shared host or storage interference.

Resource samples report container working memory, not process RSS or exact CPU
seconds. PostgreSQL quota bytes are logical usage; table/index/TOAST allocation
and WAL are separate physical costs. Repeated padding can compress heavily, so
high-entropy retained-padding cases are reported separately. Per-case database
high-water growth and WAL deltas can include setup and final checks.

## Qualification status

The original workload screens and selected boundary repeats are complete.
Sustained hot-workspace and 100-workspace cases, mature rolling-window repeats,
distinct-group batch controls and the longer paired aged-store check are pending.
Final tables and the sanitized aggregate will be added after those runs finish.
