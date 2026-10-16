# PostgreSQL queue and commit follow-up

October 16, 2026 (UTC). **Shorter transactions improved queueing under pressure.
Low-load tail latency remains variable. SQLite stayed fast.**

## Why small requests previously looked slower

A workspace serializes its state changes through durable commit. Extra database
round trips lengthen that critical section; a brief slowdown can leave a queue
that lasts much longer than the original stall.

The earlier four-CPU small-request comparison averaged 5.7 → 16.2 ms in commit
and 1.3 → 406.8 ms waiting for the writer, even though the write callback improved.
The two-CPU comparison moved in the opposite direction for average commit time.
Those results do not establish a consistent regression from entity batching.

This investigation also reproduced variation with unchanged code: the initial
100/s baseline repeats measured 359 and 311 ms p99 across ten workspaces. Spreading
100/s across 100 workspaces measured 101 ms. After redeployment, the same baseline
measured 90 ms, with much lower private-network SQL round-trip time. Comparisons
across those deployments cannot isolate a code improvement.

## What changed

Three focused Go changes eliminate **five PostgreSQL round trips per push**:

- Acquire workspace, grant and key authorization locks in one dependent query.
- Save receipt, latest receipt and durable usage in one statement.
- Append audit and apply its retention boundary in one statement.

Permission decisions, expiry checks, evaluation and quota policy remain in Go.
The transaction still checks ownership and commits durably before acknowledging.
SQLite retains its existing SQL paths. New bounded metrics measure writer hold,
transaction authorization and preparation for commit. Lab diagnostics record WAL
I/O timing, wait events and MultiXact counters without recording queries or secrets.

## Matched Railway results

Both binaries run in the **same app deployment**: previous batched implementation
`26bb27d`, candidate `a97b769`. One app has 2 CPU/2 GB; PostgreSQL uses 2 CPU/4 GB
or 4 CPU/8 GB. A separate generator has 2 CPU/1 GB. Direct Go pool: 16 connections,
`exec` mode, verified TLS, private networking, and fsync/synchronous commit/full-page
writes enabled. Seed 42, simultaneous inspection reads and ten workspaces unless
stated. Sizes are configured limits. All rows below are 30-second screens.

| Workload | Database CPUs | Accepted before → after | p99 before → after |
| --- | ---: | ---: | ---: |
| 10 KB, 100 requests/s | 2 | 3,000 → 3,000 / 3,000 | 90 → 93 ms |
| 10 KB, 250 requests/s | 2 | 4,148 → 4,359 / 7,500 | 3,523 → 3,778 ms |
| 10 KB, 250 requests/s | 4 | 7,500 → 7,500 / 7,500 | 409 → 303 ms |
| Mixed JSON, 50 requests/s | 4 | 1,500 → 1,500 / 1,500 | 276 → 209 ms |
| Mixed JSON, 100 requests/s | 4 | 3,000 → 3,000 / 3,000 | 861 → 308 ms |
| 600 KB, one observation, 100 requests/s | 2 | 3,000 → 3,000 / 3,000 | 200 → 155 ms |

At 250 small requests/s with four database CPUs, average writer wait fell
**34.4 → 13.6 ms (60%)** while average commit stayed **6.2 → 6.3 ms**. Database CPU
was about 3.05 cores in both runs; app CPU fell 0.65 → 0.61 cores. At 100/s,
measured SQL calls fell about **45 → 40 per request (11%)**, including background
and inspection traffic. Fewer round trips created room in the workspace queue;
these changes did not reduce the durable flush cost.

The two-CPU database saturated at 250/s. Four CPUs allowed complete acceptance,
but 303 ms still misses the chosen **250 ms p99** target. Memory stayed below the
limits; this small dataset does not demonstrate a benefit from more RAM.

Results varied by workload. The low-rate 100-observation batch improved
572 → 173 ms with two database CPUs, then worsened 336 → 407 ms in the reversed
four-CPU comparison. Alert p99 worsened 600 → 702 ms. Retained 100 KB data improved
1,226 → 1,015 ms but still missed the target. These are not universal latency gains.

Three short 100/s screens per version on four database CPUs measured p99
**75 / 132 / 81 ms before** and **93 / 89 / 128 ms after**. All passed. The medians
were 81 and 93 ms and the ranges overlap; these runs establish no consistent
low-load tail-latency improvement.

The original longer qualification also passed: three 180-second candidate runs
at 35/s across 100 workspaces accepted all **18,900 pushes**, with p99
**91 / 80 / 78 ms**. The matched baseline accepted its 6,300 pushes at **89 ms**.
These bounded runs establish repeatable acceptance at the tested rate and duration;
they do not establish a maximum or long-duration production operating rate.

SQLite passed all three matched controls: small requests at 250/s, batches at 5/s
and mixed traffic at 50/s. Their p99 values were **17 → 14**, **69 → 47** and
**40 → 36 ms**. Its persistence implementation was unchanged.

## Verification and limits

- **51 cases:** the same 43-case campaign plus four unchanged-code diagnostics
  and four focused small-request repeats. [Full CSV](../../benchmarks/push/results/2026-10-16-railway-queue.csv).
- **130,958 timed Ding pushes** accepted from 140,400 offers, with every accepted
  receipt checked against durable state. Overload rejections remain in the results.
  Three separate generator calibrations account for another 36,000 responses.
- **20,496 inspection reads**, zero correctness errors, and clean replay,
  accounting, authorization, telemetry, resource-limit and graceful restart checks.
  Alert fixtures add 30 recovery pushes each after timing; their requests, bytes
  and observations were reconciled separately from offered-load totals.
- [Linux CI passed](https://github.com/ding-labs/ding/actions/runs/38838859431),
  including real PostgreSQL contracts and race detection. Local race suites passed
  too, including permission changes during a blocked admission, receipt rollback,
  audit retention rollback and exact logical-byte accounting.

Operation averages use sampled counter deltas and include background work. The
SQLite controls use the app filesystem; PostgreSQL uses its Railway volume.
Their absolute latencies are not a matched-storage comparison.

The original cloud soak continues on its original deployment. Local test databases
and the owned Railway databases/roles were removed; the existing lab PostgreSQL
service was returned to its previous 1 CPU/2 GB allocation. Both test deployments
were stopped and their secrets cleared. Railway's approval layer refused permanent
deletion, so the two empty service records remain. Compressed evidence is preserved
privately. These short, fresh fixtures measure one active Go server and one
PostgreSQL server; production HA,
multi-server operation and external-provider delivery retain their separate gates.
