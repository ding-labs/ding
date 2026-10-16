# Linux qualification for Ding Cloud PostgreSQL

Execution specification for the [Cloud implementation plan](cloud-postgres-plan.md),
October 15, 2026. The primary question is the safe operating capacity of **one Go
server plus one PostgreSQL server**, including reads, workers and retention.
These are proposed experiments and engineering targets, not measured capacities.

## Environment and comparison controls

Use dedicated Linux machines or VMs with a recorded CPU architecture, filesystem,
storage class, kernel and resource limits. Keep the generator on a separate machine
or verified isolated cores. Use pinned native-architecture builds. Confirm no CPU
steal, swapping or competing workload invalidates a capacity run.

1. Run a small native Linux versus Docker-on-Linux control with equivalent CPU,
   memory, storage and durability. Collect cgroup throttled time, I/O latency and
   actual utilization. This isolates container overhead from application changes.
   Docker CPU quotas can throttle fractional allocations; record the period/quota
   explicitly. [Docker resource controls](https://docs.docker.com/engine/containers/resource_constraints/).
2. Use same-host loopback only as a diagnostic control. Qualify the actual one-app,
   one-database topology on separate Linux hosts with verified TLS. Record measured
   network RTT; separately test approximately 0.5, 2 and 10 ms round-trip latency
   to expose sensitivity to sequential queries. Verify induced RTT rather than
   assuming a traffic-control setting equals round-trip delay.
3. Keep PostgreSQL `fsync`, `full_page_writes`, `synchronous_commit` enabled and
   SQLite WAL/FULL. Record buffer, checkpoint, autovacuum and pool configuration.
   Test warmed and cold/recovery behavior separately. Database files use owned
   Linux storage; keep generator journals off the measured database volume.
4. Run an isolated `pgbench` sanity control and a representative Ding SQL trace.
   This diagnoses database/host limits; raw database TPS is not Ding push capacity.
   [pgbench documentation](https://www.postgresql.org/docs/17/pgbench.html).

Proposed resource profiles, to be validated rather than advertised as requirements:

| Profile | Go server | PostgreSQL server | Question |
| --- | --- | --- | --- |
| Initial Cloud | 1 vCPU / 1 GiB | 1 vCPU / 2 GiB | What can an inexpensive two-service setup sustain? |
| More app CPU | 2 vCPU / 2 GiB | 1 vCPU / 2 GiB | Is Go the bottleneck? |
| More database | 1 vCPU / 1 GiB | 2 vCPU / 4 GiB | Does independent database sizing help? |
| Larger pair | 2 vCPU / 2 GiB | 2 vCPU / 4 GiB | What does the next capacity tier buy? |

Run SQLite controls at its own lean 1 vCPU / 1 GiB profile and at the same total
resources as the initial Cloud pair (2 vCPU / 3 GiB). Label those comparisons
separately. At zero/low load, test smaller memory allocations as an additional cost
experiment. Report configured allocations, actual idle use, connection count and
storage growth. Compute monthly cost only from the deployment's current prices,
including storage, backups and network; no hosting purchase is part of this plan.

## Required workloads

Extend `benchmarks/push` and `internal/cloud/scale_lab_test.go`. Retain real HTTP,
authentication, Go evaluation, durable acknowledgement and ledger reconciliation.
Declare independent test users, workspace/watch distribution, exact body bytes,
projected observation count and retained bytes in every manifest.

| Family | Cases and purpose |
| --- | --- |
| Idle overhead | 10/100/1,000 push-only workspaces, no due timers, empty outboxes; then a separate due-timer/acquisition case. Validate the five-transactions/s idle target without delaying work. |
| One-server baseline | Small and 10 KB bodies, one output, 1/10/100 workspaces; screen 25/50/100/250/500/1,000 pushes/s, stop at overload and refine the boundary. |
| Payload and fanout | 100 KB/600 KB/1 MiB bodies; 1/18/100 outputs, repeated versus distinct groups, nested input, retained versus discarded high-entropy padding. Use a selected representative set rather than every combination. |
| Established customers | 10,000 groups, 100,000+ receipts, mature windows, pinned evidence, expiry and active retention. Include a dataset larger than configured database cache. |
| Mixed service | 100 and 1,000 workspaces; 80% small one-output requests, 15% 100 KB/18-output, 5% 600 KB/100-output. Add 20 authenticated inspection requests/s, 100 HTTP watches at 60-second intervals, timers and delivery. This is a declared synthetic mix, not observed customer traffic. |
| Fairness and delivery | One noisy customer with quiet peers; healthy versus eventful input; fast, 100 ms and unavailable receivers; one ordered stream and many streams. Report quiet-customer latency and backlog age. |
| Protection and recovery | Default policy versus raised finite lab limits; binding quotas, bad credentials, expired grants, large/deep JSON, queue/pool/disk pressure and recovery. Expected policy rejections are separate from valid-input capacity. |

The existing Cloud soak's roughly 600-byte, 18-output request once per minute is
a correctness workload. A 600 KB case is about a thousand times larger and must
have its actual byte count recorded. HTTP requests/s, observations/s and MB/s
must always be reported together.

## Execution order and pass conditions

- **Before each optimization:** reproduce its affected case on Linux, then compare
  before/after at identical settings. Change one variable. Run common SQLite and
  PostgreSQL contracts, plus targeted race tests for changed concurrency. Reject
  correctness failures and investigate repeatable regressions beyond 10% in SQLite
  latency/throughput or idle CPU/RSS; document variance and any accepted tradeoff.
- **Screens:** 30-second rate ramps locate the limit. A saturated generator, missing
  resource samples or unexpected host contention makes a run inconclusive. Exclude
  a fixed-arrival run when generator scheduling p99 exceeds 20 ms (8% of the
  acknowledgement SLO); record host CPU steal/I/O wait and generator throttling
  to distinguish noisy runners from application limits. Preserve those results
  as diagnostics rather than interpreting them as a clean capacity comparison.
  Exclude
  profiling from capacity runs. Alternate baseline/candidate order where practical.
- **Qualification:** three 180-second repeats at the candidate boundary, then
  30-minute hot-workspace and many-workspace cases at qualified rates. Include a
  common rate for before/after comparisons. Require every repeat to pass.
- **Operating rate:** begin at 70% of the highest repeatably passing rate for each
  workload; verify it under the mixed workload and routine maintenance. This is
  rate headroom, not a claim of 30% free CPU. Publish a lower rate if needed.
- **Final operations run:** one six-hour aged-store/mixed-service run on the final
  candidate, including backup and retention, with storage slopes and final recovery.
  Record retention settings and which real maintenance cycles elapsed. Longer
  retention guarantees remain untested if the run does not cover them.

Valid-input capacity requires p99 acknowledgement below 250 ms from planned
arrival, zero missed arrivals, unexpected rejections, uncertain responses or read
errors, and exact durable reconciliation. The historical 99.9% first-attempt
threshold is a floor; it does not override those zero-error gates. Report HTTP
and generator scheduling latency separately. Require due-work p99 within one
second, no steadily growing backlog under a healthy receiver, and recovery to
baseline after the declared burst/outage. Intentional fault tests use their own
recovery assertions and do not count as successful steady-capacity runs.

Record CPU/RSS/heap, GC, SQL calls per accepted request, transaction volume, pool
waits/churn, row-lock waits, query plans, WAL/fsync/checkpoints, autovacuum/dead
tuples, storage slope, worker lag and delivery age. Keep counters inside actual
measurement windows. Reconcile offered keys, receipt spans, observations, events,
outbox and durable usage after every run and after restart.

## Multi-server and reliability gates

After distributed coordination is implemented, run 1/2/4 Go processes against
one database behind a load balancer. Compare fixed total app resources first,
then added app resources with fixed database resources, then database growth.
Connection limits must include every node, ownership/notification connection and
operator reserve. Verify customer limits remain constant as node count changes.

Use deterministic barriers for duplicate/conflicting pushes on different nodes,
revocation/deletion on one node while another accepts, concurrent timers and
ordered delivery. Kill owners during acquisition, commit and delivery; delay
renewal, drop notifications and partition a node from the database. Stale owners
must be fenced, accepted data must reconcile, and work must resume within the
declared lease/retry bound. Verify authentication and tenant isolation through
every node, including pooled connection reuse.

On disposable fixtures, test PostgreSQL restart, client disconnection after commit,
disk pressure, encrypted backup restore and WAL/PITR recovery. Measure the proposed
30-minute recovery / five-minute storage-loss objectives on recorded dataset sizes.
A process kill does not prove physical power-loss durability. Test managed failover
separately if that deployment is later chosen. [PostgreSQL PITR](https://www.postgresql.org/docs/17/continuous-archiving.html).

## Deliverables, cleanup and stopping rules

Publish a short table for each qualified profile: safe pushes/s, observations/s,
MB/s, customer/read load, p99, storage/day, allocated/idle resources, recovery time
and cost when known. Explain the first limiting component and the measured gain
from the next size or server count. Scale Go for sustained Go CPU/queue pressure;
scale/tune PostgreSQL for its CPU, I/O, memory or lock pressure. More Go servers
cannot repair a saturated database or a serialized hot workspace.

Preserve manifests, journals, reconciliations, metrics and profiles. After export
and audit, automatically delete only owned fixture databases, volumes, networks
and credentials. Default failed-state retention is a private bounded snapshot
with a stated expiry/disk budget; never global-prune Docker. Stop further repeats
once a failure's cause is established, fix it, then rerun the affected cases and
the final fixed regression set. Do not spend hours extending an already failed rate.

Leave the active Cloud soak, its credentials, Worker and Railway deployment
unchanged. All new load, fault injection and database resources belong to this
isolated Linux qualification.
