# Railway: one Ding server and one PostgreSQL server

October 15, 2026. These are measurements of the current implementation, not a
universal limit on PostgreSQL or a production capacity promise.

## What we know

**More resources help, but reducing database calls is the next major opportunity.**
The app and database run as separate Railway Linux services in the same region,
using private networking and verified TLS. SQLite and the production Cloud soak
were unchanged. A separate generator is outside the app/database resource budget.

| Change | Observed result | Interpretation |
| --- | --- | --- |
| Database: 1 CPU/2 GB → 2 CPU/4 GB; mixed JSON at 50 requests/s | p99 fell from 4,291 to 416 ms; all 1,500 requests accepted in both screens | Removing database CPU throttling helped substantially |
| Database: 2 CPU/4 GB → 4 CPU/8 GB, same workload | p99 416 → 486 ms | More database CPU alone did not fix this workload |
| Go connection pool: 8 → 16; 10 KB JSON at 250/s | 7,032/7,500 accepted and p99 2,086 ms → 7,500/7,500 and 163 ms | Pool sizing mattered; 32 connections then worsened p99 to 447 ms |
| Database: 4 CPU/8 GB → 8 CPU/16 GB; 10 KB JSON at 500/s, pool 32 | 9,475/15,000 → 11,294/15,000 accepted | More throughput under overload, but neither configuration qualified |

These are **30-second screens**, mostly with 10 workspaces. They identify useful
directions, not sustained operating rates or isolated effects of RAM. Memory use
was far below the larger limits; this small, fresh dataset did not establish a
benefit from extra RAM. Sizes are configured limits, not dedicated CPU reservations.

### App CPU also matters for large bodies

A fixed-deployment 4 → 2 → 4 app CPU control sent 600 KB JSON, one observation per
request, at 100 requests/s. PostgreSQL stayed at 8 CPU/16 GB, pool 16:

| App allocation | Accepted / offered | p99 |
| --- | --- | --- |
| 4 CPU/4 GB | 3,000 / 3,000 | 526 ms |
| 2 CPU/2 GB | 1,771 / 3,000 | 6,989 ms |
| 4 CPU/4 GB, repeated | 2,767 / 3,000 | 4,218 ms |

The four-CPU cases used about 2.3 app cores, supporting a benefit from more app CPU.
However, **100/s with 600 KB bodies is not a qualified operating rate**. The repeat
varied considerably. A separate no-op transport calibration passed 100/s (60 MB/s);
250/s (150 MB/s) missed the generator schedule and is excluded from capacity claims.

### Sustained qualification

The fixed deployment uses app 2 CPU/2 GB, PostgreSQL 4 CPU/8 GB, pool 16 and 100
workspaces. Each push contains 10,000 bytes of nested JSON and one observation.
Inspection reads run concurrently. A 30-second screen at 50/s passed at 150 ms
p99, but the first 180-second repeat accepted all 9,000 requests with **446 ms
p99**, failing the 250 ms latency target. Its 1,797 reads had no errors; receipt,
usage, replay, authorization and graceful restart checks passed. Average measured
CPU was 0.55 app cores and 1.01 database cores, so this failure was not CPU saturation.

Lower-rate repeats and a sustained run are in progress. No sustained Railway
operating rate is qualified yet. The earlier Linux loopback result—35/s for 30
minutes with 55-byte bodies—does not substitute for this separate-service test.

## What the realistic workloads revealed

The synthetic business mix contains 80% 10 KB/one-observation requests, 15%
100 KB/18-observation batches and 5% 600 KB/100-observation batches. Bodies contain
nested workflow metadata and reproducible high-entropy padding. Exact weights
apply to complete cycles; the evidence records actual counts for partial cycles.

Batch fanout costs more than body size alone. A 600 KB/100-observation screen at
5 requests/s accepted all 150 requests but had 699 ms p99. During a measured
25-second interval, 125 such requests coincided with roughly **450 SQL statements
per request**, including background and inspection work. Go evaluation itself
averaged about four microseconds per observation. Entity state and retention pins
still perform repeated selects, deletes and inserts inside the write transaction.

A corrected alert-delivery control accepted 150 mixed requests, reconciled all
2,076 distinct `(workspace, event)` deliveries and drained the outbox to zero.
Its p99 was 2,806 ms, so delivery correctness passed while the latency gate failed.
Eight of the ten configured workspaces received traffic in that partial cycle.
The receiver is a fast local fixture, not an external webhook provider.

## Why the results vary

Private networking removes the public route; it does not remove database round
trips. Observed `SELECT 1` median latency ranged from about 0.3 to 2.65 ms across
deployments/times. Even the fixed deployment varied. Those numbers include SQL
and client overhead. Hundreds of sequential calls amplify small delays.

The collector now records deployment identity and a 100-query SQL round-trip
baseline before every case. Cross-deployment improvements cannot be attributed
solely to CPU. The final resource control and qualification use app deployment
`be64def2-efff-45f4-b83f-d89613a57d42`, source `db0594d`.

## PgBouncer and next improvements

Ding already shares a bounded Go connection pool. PgBouncer can help manage
connections as application instances multiply, but it does not eliminate the
SQL round trips measured here. Increasing our pool helped up to a point; more
connections subsequently made latency worse.

Transaction pooling is **not currently a drop-in configuration**: Ding's ownership
guard needs a persistent session advisory lock and backend identity. It would
need a separate direct ownership connection configuration before query connections
could safely use transaction pooling. PgBouncer was not deployed in this experiment.
See [pooling features](https://www.pgbouncer.org/features.html) and
[configuration](https://www.pgbouncer.org/config).

The next focused changes should batch entity/pin persistence, reduce repeated
authorization/fencing/inspection queries without weakening their guarantees, and
shorten workspace write transactions. Then repeat this fixed-placement matrix.
Large aged datasets, HTTP polling mixed with push, real external delivery,
multi-server coordination, physical crash recovery and backup/PITR objectives
remain separate qualification gates.

## Evidence and safeguards

The generator now persists the arrival plan before measurement and journals
responses asynchronously through a bounded queue. Lateness remains a failed gate.
Every accepted receipt is checked against durable usage and observations, followed
by replay/conflict, authorization and graceful restart checks. Capacity requires
all planned first attempts accepted, no unexpected errors, scheduling p99 ≤20 ms,
acknowledgement p99 <250 ms, complete metrics/resources and no pending deliveries.

Early alert harness failures are excluded: restart snapshots raced active delivery
workers; a restart restored the fixture's destination rate limit; and receiver
identities omitted the workspace. Those harness issues were corrected and tested.
Failed-state archives were preserved privately before their owned databases were
removed. The runtime runs in a Go test process with expanded lab admission limits;
this does not establish capacity under production policy defaults.

The [Linux results](cloud-postgres-progress.md) remain a separate comparison family.
Changes continue in [PR #58](https://github.com/ding-labs/ding/pull/58), on the
existing PR stack, with the small commits preserved.
