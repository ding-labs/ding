# Railway write-batching comparison

October 16, 2026 UTC. The four-CPU comparison is still in progress. These are
completed 30-second screens, not sustained production capacity claims.

## What changed

The Go evaluator still processes observations and emits events in order. It now
persists each group's final state once per acceptance transaction. PostgreSQL
writes up to 100 states together, synchronizes their retention references in one
set operation, and accounts their retained bytes per statement. The entity and
reference writes need two SQL round trips per chunk; other acceptance, event,
delivery and inspection queries remain. SQLite keeps its embedded SQL path.

## Two-CPU results

Ding: **2 CPU / 2 GB**. PostgreSQL: **2 CPU / 4 GB**. Pool: 16. Ten workspaces,
concurrent inspection reads, verified TLS, synchronous durable commits. The old
and new executables ran in the **same deployment**, with fresh databases per case.

| Workload | Requests/s | Accepted before → after | Push p99 before → after |
| --- | ---: | ---: | ---: |
| 600 KB, 100 observations | 5 | 150 → 150 / 150 | 1,474 → 755 ms |
| 100 KB, 18 observations | 25 | 750 → 750 / 750 | 906 → 340 ms |
| Mixed JSON and batch sizes | 50 | 1,500 → 1,500 / 1,500 | 1,715 → 1,109 ms |
| 10 KB, one observation | 100 | 3,000 → 3,000 / 3,000 | 667 → 1,061 ms |
| 10 KB, one observation, overload | 250 | 3,329 → 3,195 / 7,500 | 4,867 → 5,681 ms |
| Retained 100 KB JSON | 25 | 750 → 750 / 750 | 1,143 → 1,063 ms |
| Mixed JSON with alert delivery | 5 | 150 → 150 / 150 | 3,734 → 1,987 ms |
| 600 KB, one observation | 100 | 3,000 → 2,849 / 3,000 | 1,197 → 3,438 ms |

The largest batch reduced observed SQL statements from **468 to 71 per request**
(including background/inspection work). Its average transaction callback fell
from **1,205 to 68 ms**, and database CPU from **0.937 to 0.241 cores**. This is a
substantial reduction in work, even though commit-time spikes kept p99 above the
250 ms target. The 18-observation case reduced database CPU from 1.056 to 0.593
cores; the mixed case from 1.402 to 0.935 cores.

**Single-observation throughput did not improve in these screens.** The new
version accepted about 4% fewer requests in the 250/s overload case and about 5%
fewer large bodies at 100/s. These results remain in the comparison. Additional
CPU and reversed-order controls will distinguish repeatable effects from timing
variation. All PostgreSQL screens above missed the chosen latency target.

Every accepted request passed receipt, usage, provenance, replay, authorization
and graceful restart reconciliation. Alert runs each verified **2,361 distinct
deliveries**, with no pending outbox rows. Rejected overload requests are counted
separately. No response uncertainty or correctness errors were reported.

### SQLite stayed fast

The same Linux Ding service, two CPUs, payloads and reads:

| Workload | Requests/s | p99 before → after | Result |
| --- | ---: | ---: | --- |
| 10 KB, one observation | 250 | 13 → 13 ms | All 7,500 accepted each |
| 600 KB, 100 observations | 5 | 44 → 47 ms | All 150 accepted each |
| Mixed JSON and batch sizes | 50 | 34 → 32 ms | All 1,500 accepted each |

All six runs passed the strict latency, generator, reconciliation, read and
restart gates. These short controls show no material latency regression; they do
not establish SQLite's maximum throughput or end-user disk performance.

## Controls and evidence

- Baseline: `db0594d`; candidate application: `26bb27d`. The latter includes the
  batching changes through `ca0360a`. Both use the same Go compiler/image.
- App deployment: `35a515ca-8671-41f5-aa16-46d3de1a8b87`. No redeployment between
  variants. Results record both binary hashes, resource limits and deployment ID.
- Generator: separate 2 CPU / 1 GB service. Calibrations passed at 1,000/s for
  10 KB, 100/s for 600 KB and 100/s for mixed bodies; scheduling p99 was 1 ms.
- Mixed requests contain nested metadata and high-entropy padding: 80% 10 KB /
  one observation, 15% 100 KB / 18 observations, 5% 600 KB / 100 observations over
  complete cycles. Seed 42 interleaves arrivals; partial cycles have actual counts
  in the evidence. Padding is retained only in the explicit retained case.
- SQL RTT medians were roughly 2.4–2.6 ms. Fixed placement does not eliminate
  temporal variation, CPU throttling or commit latency spikes.
- [Aggregate data](../../benchmarks/push/results/2026-10-16-railway-batching.csv).
  Private request/receiver journals, reports, metrics and manifests are preserved
  under `~/.config/ding-cloud/local-scale/railway-batch-20261015/`.
- [Linux CI passed](https://github.com/ding-labs/ding/actions/runs/38827301962),
  including real PostgreSQL migrations/accounting, buffered acceptance, rollback,
  restart, tenant isolation and race-enabled contracts. Local race suites passed
  too. The owned local PostgreSQL container/volume and credentials were removed.
- The independent production soak was left running on its pinned commit. No
  production faults, migrations, credentials or deployments were changed.
