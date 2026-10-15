# Single-server PostgreSQL: Linux progress

October 15, 2026. This is an **interim engineering result**, not a production
capacity claim. SQLite remains the default. PostgreSQL still has one active
execution owner; multi-server coordination and Cloud migration are separate gates.

## What improved

| Check | Earlier result | Current evidence |
| --- | --- | --- |
| 100 idle PostgreSQL workspaces | Repeated worker database reads | Zero runtime transactions in the latest 75-second window; maintenance about 4 transactions/s |
| 1,000 idle PostgreSQL workspaces | 31 runtime transactions/s in the first cache implementation | Zero runtime transactions; maintenance about 40 transactions/s |
| 100 workspaces, 25 small pushes/s | PostgreSQL p99 3,320 ms | Worker/query candidates: 25–113 ms across short runs |
| Same 25 pushes/s, SQLite control | p99 7 ms | p99 7–8 ms |

The idle result counts real SQL transactions. It does not mean the database does
no work: minute maintenance is measured separately. The latency figures came from
separate GitHub VMs, so they demonstrate progress without establishing a precise
speedup multiplier. Small requests were **55 bytes with one observation**, not
600 KB. Each 30-second, 25/s case offered 750 requests and reconciled its receipts.

Changes include shared worker snapshots, avoiding empty delivery writes, fewer
transaction setup queries, joined watch reads, batching observation inserts and
accounting, and retaining the configured number of idle connections. Go still
performs projection, evaluation, authorization and scheduling. Final byte quota
checks now avoid counting unrelated watches/deliveries. SQLite keeps its page-based
quota; PostgreSQL keeps its existing logical-byte definition.

## What the faster screens do not establish

**100 small pushes/s is not qualified on the initial one-CPU database profile.**
Some short runs passed; others saturated database CPU, accumulated latency and
rejected requests. App CPU had headroom. Pool-size comparisons on different VMs
are not enough to choose an optimum. Prepared-statement caching remains optional.

An experimental combined group-reference query was removed. Its suspected CPU
regression was not confirmed by the rerun. A same-machine A/B/A control then measured 100 requests/s across 100 workspaces:

| Database allocation | p99 | Accepted / offered |
| --- | --- | --- |
| 1 CPU / 2 GiB | 108 ms | 3,000 / 3,000 |
| 2 CPU / 4 GiB | 18 ms | 3,000 / 3,000 |
| Back to 1 CPU / 2 GiB | 128 ms | 3,000 / 3,000 |

The Go server stayed at 1 CPU / 1 GiB. All three 30-second cases reconciled and
passed the screen. In comparable resource-sample intervals, the small database
hit its CPU quota in 27 and 30 periods; the larger database hit it zero times.
This supports a benefit from independent database sizing and burst headroom,
not a sustained throughput multiplier. A separate final hot-workspace stage had
a late generator and is inconclusive.

An 18-output batch screen passed after observation batching (25 requests/s,
p99 220 ms). Its earlier baseline had a late generator, so the apparent latency
ratio is not a clean comparison. 100-output overload remains unresolved. Profiling
and generator scheduling p99 above 20 ms are excluded from capacity qualification.

## Verification and next gates

Race-enabled real PostgreSQL database, store, server-state, HTTP and CLI contracts
passed after the final quota-read change. SQLite's complete watch runtime race
suite and targeted PostgreSQL runtime tests passed. All 16 Python harness tests
passed. Disposable local PostgreSQL volumes and credentials were deleted after
verification; private measurements remain available.

The first A/B/A attempt was interrupted because a Bash `SECONDS` variable caused
later stages to lengthen. Its evidence is retained and excluded. The corrected
run records exactly 30 seconds in every stage.

Three 180-second repeats at 50 requests/s are in progress for one and 100 workspaces
on both backends. The initial profile allocates Go 1 CPU/1 GiB and PostgreSQL
1 CPU/2 GiB, with four query connections. SQLite receives 1 CPU/1 GiB. Generators
run separately on the same VM; this is a loopback diagnostic topology.

Still required: sustained hot/many-workspace qualification, due-work latency,
representative mixed and aged workloads, SQL/lock/checkpoint analysis, Linux native
versus container control, separate-host TLS/RTT tests, recovery/PITR objectives and
migration rehearsal. Observation inserts are batched; event, outbox and group-state
writes still offer further optimization opportunities. No safe Cloud rate or
multi-server scaling multiplier is published yet.

## Review stack and evidence

[PR #58](https://github.com/ding-labs/ding/pull/58) builds on #57, which builds on
#56 → #55 → #54 → #53 → #52 → main. Every preceding PR head is an ancestor of this
branch. Preserve the small commits and merge in dependency order. The Cloud soak
and Railway deployment have not been changed by this work.

Evidence: [baseline](https://github.com/ding-labs/ding/actions/runs/38787585777),
[worker/query screen](https://github.com/ding-labs/ding/actions/runs/38789220528),
[idle](https://github.com/ding-labs/ding/actions/runs/38792417789),
[SQL diagnostic](https://github.com/ding-labs/ding/actions/runs/38795250725),
[resource control](https://github.com/ding-labs/ding/actions/runs/38796772072),
[longer repeats](https://github.com/ding-labs/ding/actions/runs/38796901012).
The [allowlisted measurements](../../benchmarks/push/results/2026-10-15-linux.csv)
retain passing, failed and inconclusive screens, including source revisions and
actual request sizes. The [qualification specification](cloud-postgres-linux-tests.md) defines the
remaining acceptance gates and the [implementation plan](cloud-postgres-plan.md)
separates single-server, operations, multi-server and migration work.
