# Push performance: what the tests tell us

**SQLite got faster in the workloads with lots of groups or history. Our current
PostgreSQL implementation is slower than SQLite in the tested configurations.**
SQLite remains the local default and PostgreSQL is configurable for self-hosting.
The subsequent [Cloud plan](cloud-postgres-plan.md) targets PostgreSQL after
optimization and production qualification; these measurements precede that work.

## What improved versus before?

The following comparisons use the same hardware, workload and resource budget.
“p99” means the time within which 99% of accepted requests finished.

| Workload | Before | Now | Gain |
| --- | --- | --- | --- |
| Watch with 10,000 groups, 25 pushes/s | p99 112–192 ms | 24–26 ms | About 5× faster tail latency across three trials |
| Store with 100,000 receipts, 100 pushes/s | p99 118 ms | 26 ms | About 4.5× in one three-minute pair; earlier short results varied |

**We have not proved a higher reliable maximum request rate overall.** At 500
pushes/s across ten workspaces, the old code passed 0/3 three-minute trials and
the current code passed 1/3. That rate is still unreliable on the tested one-CPU budget.

## SQLite versus PostgreSQL

Yes, PostgreSQL lost performance in these tests. One direct 30-second comparison
used ten workspaces, 100 KB JSON, 100 pushes/s and **1 CPU / 1 GiB total**:

| Backend | Requests accepted | p99 latency |
| --- | --- | --- |
| SQLite | 3,000 / 3,000 — 100% | 11 ms |
| PostgreSQL, pool 8 | 1,900 / 3,000 — 63% | 9.2 seconds |

PostgreSQL split that budget equally between Go and the database. Later tuning
to pool 2 improved small-payload performance; the large-payload matrix was not
repeated with that pool setting. This measures our adapter and configuration.

## What held up for 30 minutes?

| Configuration | Result |
| --- | --- |
| SQLite, 100 workspaces, 4 CPU / 4 GiB total | **250 pushes/s**, all 450,000 accepted; p99 **65 ms**; reads and reopen passed |
| PostgreSQL, one workspace, 1 CPU / 1 GiB total, pool 2 | **50 pushes/s**, all 90,000 accepted; p99 **117 ms**; reads and reopen passed |

These are different workloads and budgets. PostgreSQL did not establish a
repeatable 100-workspace latency result even at 10–25 pushes/s on the tested
2/4-CPU budgets, so its cohort long run was skipped.

SQLite also ran 250/s into one workspace for 30 minutes on one CPU, accepting
99.984% of inputs, but 70 throttles and one temporary 503 failed the zero-error
target. We cannot call that a clean sustained rate.

## What this means

- **The gains are real for larger group state and receipt history.** They are not
  evidence of a universal throughput increase.
- **This PostgreSQL implementation did not establish a performance upgrade.**
  The revised Cloud plan addresses its bottlenecks before a production migration.
- Before a PostgreSQL cloud migration, investigate its query/transaction volume
  and background work. At 25 pushes/s across 100 workspaces, we measured about
  647 database transactions/s including authentication, reads and background work.
  We have not isolated each source of that cost. A larger connection pool did
  not consistently help.

Core monitoring logic remains in Go. PostgreSQL is configurable for fresh
self-hosted installations with one active server. Horizontal operation, poolers
and existing SQLite data migration still need separate work.

[Full measurements and test limits](push-scale-results-appendix.md) ·
[191 case records](../../benchmarks/push/results/2026-10-15-qualification.csv) ·
[Database configuration](../operate/databases.md)
