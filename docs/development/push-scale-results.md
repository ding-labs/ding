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

## Completed boundary and control measurements

These rows use 1 CPU / 1 GiB total and profiles off. A three-minute trial is
still a bounded local experiment, not a production service-level guarantee.

| Workload | First-attempt acknowledgements per 180 s trial | Acceptance p99 per trial | Qualified trials |
| --- | --- | --- | --- |
| SQLite, one workspace, 500 requests/s | 90,000; 89,774; 89,810 of 90,000 | 117; 191; 420 ms | 1/3; the latter trials throttled 226 and 190 requests |
| SQLite, one workspace, 750 requests/s | 134,239; 133,589; 134,335 of 135,000 | 188; 249; 239 ms | 0/3; all throttled |
| PostgreSQL, one workspace, 25 requests/s, pool 8 | 4,500 of 4,500 each | 230; 113; 41 ms | 3/3 |
| PostgreSQL, one workspace, 50 requests/s, pool 8 | 9,000 of 9,000 each | 402; 316; 587 ms | 0/3; latency exceeds target |
| PostgreSQL, 10 workspaces plus inspection, 25 requests/s, pool 2 | 4,500 of 4,500 each | 38; 28; 29 ms | 3/3; read p99 10; 8; 8 ms |

The three matched 10,000-group trials offered 25 requests/s with one observation
per request. Control p99 was **112 / 133 / 192 ms**, candidate
**24 / 25 / 26 ms**. Every trial acknowledged all 4,500 inputs and reconciled.
This is a repeatable improvement for that healthy grouped-input workload.

Other controls do not support a universal speedup: the 30-second one-workspace
250/s pair measured control 93 ms versus candidate 270 ms; the ten-workspace
500/s pair measured 10 ms versus 579 ms. Both acknowledged every request, but
those candidate screens failed the latency target. Earlier candidate screens were
faster, so these differences require repeated controls before attribution.
The 100,000-receipt paired screen similarly measured 15 ms versus 433 ms, while
the primary candidate screen measured 15 ms. Its longer paired check is pending.
The approximately 1.23 ms to 7.3–7.7 microsecond receipt-count query improvement
is a query microbenchmark; it cannot establish an end-to-end throughput gain.

A larger connection pool did not consistently improve PostgreSQL: at 25/s across
10 workspaces, 30-second pool 1 / 2 / 4 / 8 screens measured ingestion p99
26 / 28 / 181 / 1,210 ms. Pool 2 then passed the three repeated trials above.
The larger-budget 100-workspace screens passed at 50/s on both 2 CPU / 2 GiB and
4 CPU / 4 GiB, with p99 132 and 18 ms respectively. These are separate budgets;
they do not qualify the 1 CPU configuration.

The 1 CPU PostgreSQL 100-workspace cases could not finish their live postchecks
within the bounded test timeout. They remain inconclusive for capacity. Offline
audits recovered all 828, 816 and 737 acknowledged receipts in the three preserved
attempts and reconciled identities, digests, observation provenance and durable
usage. One additional committed request had lost its HTTP response. These audits
do not replace the failed live observer or upgrade the capacity verdict.

## Sustained measurements

The first 1,800-second run used one SQLite workspace, 250 planned requests/s,
concurrent inspection and final reopen. It acknowledged **449,929 / 450,000**
requests (99.9842%), with p95 **12 ms**, p99 **163 ms**, 17,978 completed reads
and read p99 **6 ms**. Every acknowledged input reconciled; there were no read or
clock errors, and reopen/postchecks passed.

It **did not qualify** under the zero-rejection rule: 70 requests were throttled
at 13:30 UTC and one returned temporary `acceptance_uncertain` at 13:40 UTC.
The uncertain response had no durable receipt; reconciliation found no missing
acknowledged input. All throttles occurred about 24 seconds after the other
project's container restart. That temporal overlap is a confounder, not proof of
causation. This run cannot establish an uncontended sustained 250/s service rate.

The SQLite allocated database grew by approximately **660 MB** over the accepted
449,929 requests, roughly 1.47 KB per request for this tiny-body workload including
its durable receipt/state/index costs. The run uses the raised lab storage cap;
the production default 128 MiB envelope would bind much earlier.

## Delivery screens

At 25 requests/s for 30 seconds, both backends acknowledged every input with a
separate slow local receiver. One ordered stream still had 2 SQLite / 7 PostgreSQL
pending deliveries after the one-minute drain bound. Four independent streams
with four workers drained completely in about one second after input ended
(SQLite p99 23 ms; PostgreSQL p99 156 ms). Receiver journals reconciled durable
delivery identities, including warmup and postcheck deliveries. Fast and local
outage/recovery screens also drained on both backends. These are local webhook
experiments; external provider delivery remains a separate release gate.

## Storage and remaining costs

Retaining arbitrary JSON needs a separate storage budget from parsing and then
discarding padding. In the 30-second retained-payload screens, PostgreSQL's
observed database high-water growth was:

| Incoming body and offered rate | Repeated padding | High-entropy padding |
| --- | --- | --- |
| 100,000 bytes, 25 requests/s | 13.7 MB | 82.8 MB |
| 600,000 bytes, 5 requests/s | 11.9 MB | 93.7 MB |

These are sampled whole-case allocation changes including setup/checks, not exact
per-tenant retained byte totals. All requests acknowledged and reconciled; the
repeated 100 KB PostgreSQL screen nevertheless failed latency at p99 276 ms.
Logical quota usage grew similarly within each repeated/entropy pair, while
physical growth differed substantially. Compressible fixture strings must not
stand in for arbitrary user data. Sampled PostgreSQL WAL reached approximately
185 MB in the entropy cases and needs separate reserve.

SQLite's final allocated database files were approximately 89–91 MB for the
100 KB cases and 105 MB for the 600 KB cases, plus roughly 4.3–4.5 MB WAL. Those
are final footprints including warmup and checks, rather than the PostgreSQL
growth statistic above. The aggregate retains both accounting categories.

Separate 60-second diagnostics found SQLite parser/statement execution among the
remaining CPU costs. PostgreSQL Go profiles spent substantial time in system
calls and waits; they do not contain PostgreSQL backend CPU stacks. Query round
trips, ownership checks, tenant worker overhead and database waits warrant a
focused follow-up before changing the transaction design. Pool sweeps already
show that more connections alone do not solve the bottleneck. No exact
observations-per-CPU-second figure was collected.

## Qualification status

The screening phase and selected boundary repeats have finished:

- SQLite completed all 46 catalog screens.
- PostgreSQL completed 43 distinct catalog screens. Its 100-workspace 1,000/s
  case lacked a final report; subsequent bounded retries remain inconclusive.
  The higher 2,500/s and original 500/s sustained-cohort screens were deferred.
  Lower rates and larger resource budgets are measured separately.

Sustained hot-workspace and 100-workspace cases, mature rolling-window repeats,
longer paired aged-store checks and the repeated ten-workspace control are
pending. Existing 100-output and nested 18-output screens already use distinct
groups within each batch, verified against their recorded producer bodies. Final tables and the sanitized aggregate will
be added after those runs finish.
