# Railway write-batching comparison

Follow-up: [queue and commit investigation](railway-queue-latency.md) records the
next focused changes and matched reruns, including the variable small-request tails.

October 16, 2026 UTC. The resource screens and three-minute repeats are complete.
These are measured comparisons, not production capacity promises.

## What changed

The Go evaluator still processes observations and emits events in order. It now
persists each group's final state once per acceptance transaction. PostgreSQL
writes up to 100 states together, synchronizes their retention references in one
set operation, and accounts their retained bytes per statement. The entity and
reference writes need two SQL round trips per chunk; other acceptance, event,
delivery and inspection queries remain. SQLite keeps its embedded SQL path.
PostgreSQL requires an offline migration to schema 16; SQLite's schema is unchanged.

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
fewer large bodies at 100/s. These results remain in the comparison. The
reversed-order four-CPU controls below add evidence about this result.
All PostgreSQL screens above missed the chosen latency target.

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

## Four database CPUs: substantial bulk gains

Ding stays at **2 CPU / 2 GB**; only PostgreSQL increases to **4 CPU / 8 GB**.
The same deployment and binaries remain in place. This round runs the candidate
first, then the baseline, reversing the earlier ordering.

| Workload | Requests/s | First-attempt acknowledgments before → after | p99 before → after |
| --- | ---: | ---: | ---: |
| 600 KB, 100 observations | 5 | 150 → 150 / 150 | 1,361 → 284 ms |
| Mixed JSON and batch sizes | 50 | 1,500 → 1,500 / 1,500 | 1,624 → 325 ms |
| 600 KB, 100 observations | 25 | 186 → 750 / 750 | 15,002 → 1,067 ms |
| Mixed JSON and batch sizes | 100 | 1,962 → 3,000 / 3,000 | 7,828 → 1,020 ms |
| 10 KB, one observation | 100 | 3,000 → 3,000 / 3,000 | 208 → 1,262 ms |
| 10 KB, one observation, overload | 250 | 3,601 → 3,609 / 7,500 | 4,110 → 4,690 ms |
| 10 KB, one observation, **100 workspaces** | 50 | 1,500 → 1,500 / 1,500 | 159 → 164 ms |

The 100-observation case at 5/s is **4.8 times faster at p99**, using **71% less
database CPU** (0.829 → 0.243 cores). The mixed 50/s case is **5 times faster at
p99**, with 29% less database CPU (1.312 → 0.933 cores).

At 25 batches/s, the candidate acknowledges all **75,000 observations** offered
in 30 seconds, with zero throttling or uncertainty. The baseline acknowledges
18,600 observations, throttles 139 requests and returns 425 uncertain responses.
Reconciliation finds two additional committed requests: **18,800 observations
durably stored before versus 75,000 after**. The candidate averages 0.56 app and
0.86 database cores. This establishes complete acceptance at an offered **2,500
observations/s** in this short test; its 1.07-second p99 fails the 250 ms target.
It is not a maximum or sustained operating-rate claim.

Small-webhook overload throughput is effectively unchanged between versions.
Moving the candidate database from two to four CPUs increases first-attempt
acceptance only 13% at 250/s. Transaction/commit and writer-queue timings remain
variable. The 100/s candidate run has lower average transaction callback time,
but higher commit time and writer waiting than its baseline control. More CPU
alone does not resolve this tail latency. The 100-workspace 50/s pair passes all
strict gates with similar latency; three-minute repetitions are recorded below.

The app was not CPU-throttled in the large-body control (about 1.1 of two cores
used). Memory was below the configured limits. These observations do not justify
eight database CPUs or a larger app for the measured bottleneck. RAM's benefit
for large aged datasets is untested.

## Three-minute repetitions

At **35 requests/s**, 100 workspaces and 10 KB JSON, using the same two-CPU app,
four-CPU database and pool of 16:

| Run | Accepted / offered | Push p99 | Read p99 | Strict gate |
| --- | ---: | ---: | ---: | --- |
| Batched, repeat 1 | 6,300 / 6,300 | 168 ms | 37 ms | Pass |
| Baseline control | 6,300 / 6,300 | 175 ms | 45 ms | Pass |
| Batched, repeat 2 | 6,300 / 6,300 | 174 ms | 37 ms | Pass |
| Batched, repeat 3 | 6,300 / 6,300 | 162 ms | 34 ms | Pass |

All **18,900 candidate requests** were durably reconciled, with no throttling,
uncertain responses, read errors or correctness failures. Scheduling p99 was 1 ms;
receipt, usage, replay, authorization and graceful restart checks passed. At this
rate, single-observation performance is similar to the baseline. Three short
repetitions do not establish a maximum or long-duration Railway operating rate.

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
  All 43 archives (including three generator calibrations) passed configuration,
  binary identity and planned/response ledger checks: 147,900 planned requests,
  every response recorded, and 129,881 first-attempt acknowledgments. Overload
  rejections remain in the results; archive integrity is not a performance pass.
- [Linux CI passed](https://github.com/ding-labs/ding/actions/runs/38827301962),
  including real PostgreSQL migrations/accounting, buffered acceptance, rollback,
  restart, tenant isolation and race-enabled contracts. Local race suites passed
  too. The owned local PostgreSQL container/volume and credentials were removed.
- The independent production soak was left running on its pinned commit. No
  production faults, migrations, credentials or deployments were changed.

## Cleanup

All owned Railway test databases and both lab roles were removed. Only the
existing `postgres` and `railway` databases remain. The app and generator
deployments are confirmed **REMOVED**, their saved control/database/TLS credentials
are cleared, and the local control token is deleted. PostgreSQL's original lab
allocation (1 CPU / 2 GB) was restored and verified; this was cleanup, not another
performance run. Its existing volume and deployment were preserved.

Two inactive service records remain: Railway's approval gate cancelled permanent
deletion despite the earlier user authorization. Evidence is preserved privately.
The local PostgreSQL fixture and volume were also removed. The independent Cloud
soak remains running; the latest read-only check verified 46 new samples and its
unchanged application commit, with no new failures.
