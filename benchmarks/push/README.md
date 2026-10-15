# Local push stress tests

This suite measures Ding's authenticated JSON ingestion through real local TLS,
SQLite or PostgreSQL workspace stores, evaluation, receipts, and optional webhook delivery.
Every offered key is reconciled against durable receipts, usage, and observation
rows after measurement. Capacity runs leave profiling off. Use `--profiles` for
diagnostic runs; their profiles exclude setup, warmup, and reconciliation. Compare
profiled results with unprofiled controls before making a capacity claim.

## Run cases

Run from the repository root with Go, Python 3, and Docker for container cases:

```sh
python3 benchmarks/push/run.py --environment native --cases generator-2500,one-100,one-500 --seconds 30
python3 benchmarks/push/run.py --environment container --generator-placement container --cases generator-2500,one-100,one-500 --seconds 30
python3 benchmarks/push/run.py --environment native --cases one-250,one-500 --repeat 3
python3 benchmarks/push/run.py --environment container --generator-placement container --backend postgres --pool-limit 2 --cases sustain-one --rate 50 --seconds 1800
```

`cases.json` contains payload, batch, group, receipt-aging, rolling-window,
concurrent-read, noisy-workspace, destination, and restart scenarios. Screen
briefly, then repeat boundary cases for three minutes and stable cases for thirty
minutes. Mature rolling-window measurements need more than one minute. The PostgreSQL
command above repeats the qualified hot-workspace profile; consult the results
before choosing a sustained cohort rate.

SQLite is the default; `--backend postgres` owns a pinned local PostgreSQL lab.
Container comparisons default to **1 CPU and 1 GiB total for app plus database**.
SQLite receives the whole budget. PostgreSQL splits it equally between the Go app
and database, with a shared query pool selected by `--pool-limit`. Use
`--resource-units 2` or `4` for separately labelled expanded-budget comparisons;
`--credential-readers 1/2/4/8` sweeps the SQLite control read pool.

The producer is always a separate process. `--generator-placement container`
uses a separate Linux container limited to 2 CPU/512 MiB, outside the server
budget, with actual limits checked and usage sampled. Traffic uses Linux loopback
and bypasses Docker Desktop's host port forwarding. The default `host` placement
remains useful for native exploration and historical comparisons. Keep those
network families separate; run matched controls before attributing a difference
to the database. `--receiver-process` similarly puts delivery I/O in a separate
0.5 CPU/256 MiB local TLS receiver and reconciles its durable acknowledgement log.

Databases use owned Docker volumes; evidence and producer ledgers remain on the
host filesystem. Volumes have no 1 GiB size cap. Compare growth with Railway's
actual storage allocation separately. Native apps are unlimited, so their results
are a separate exploration family. Local filesystem results cannot establish
Railway capacity.
Create the run directory's `STOP` file to finish the active case and defer the
queue, or supply a shared `--stop-file PATH` for orchestration. `run-state.json`
records completed, failed, canceled and deferred cases. Lost resource observation
or an unavailable Docker/database makes the result inconclusive and cannot
produce a stable-capacity verdict.
An interrupt cancels the active case, stops the queue and records cancellation.
Native temporary databases stay under the private case directory so an abrupt
server exit cannot erase the remaining state needed for diagnosis.
If a container exits abnormally or cannot report, its stopped database volume is
archived privately before removal. If capture fails, that owned volume is retained
and named in `execution.json`. Failed-state archives can contain local fixture
secrets and must remain private.
Ordinary Go test failures close the active workers/stores before moving fixture
state outside testing's cleanup directory. Native failures keep it in the case's
`failed-state` directory; container archives include the preserved directory.

## Interpret evidence

Private evidence defaults to `~/.config/ding-cloud/local-scale/runs/`. Each run
records its revision, executable hash, settings, execution status, resource
samples, raw ledger, and CPU/mutex/block/heap profiles. Bootstrap credentials are
generated only for local fixtures, stored privately, and removed after each case.
The launcher publishes container TLS only on loopback and accepts no cloud URL.

The default-policy case retains deployed admission/storage defaults. Other cases
raise test-only rate/concurrency limits, use a 4 GiB workspace store envelope and
one million receipts, and retain WAL/FULL durability and real authentication.
These overrides are confined to the Go test binary. Group seeding uses real
100-observation batches; measured batch sizes remain the configured workload.

Acceptance p99 starts at the planned arrival time, including client queue/ledger
delay. HTTP p99 and scheduling p99 are separate. Missed arrivals, explicit 429s,
uncertain responses and first-attempt acceptance are reported separately. A busy
generator makes the offered rate unqualified; successful reconciliation alone
does not make an overloaded workload stable.

Stability requires at least 99.9% of planned requests accepted on their first
attempt, acceptance p99 below 250 ms, successful reconciliation and postchecks,
no missed generator arrivals, unexpected rejections, uncertain responses or read
errors, and a drained outbox. These are local test targets. Restart is a
graceful store/worker reopen, not a power-loss or physical process-crash test.
Outbox drain is bounded to one minute; remaining work is reported explicitly.

```sh
go tool pprof -top -cum RUN/scale.test RUN/CASE/cpu.pprof
go tool pprof -top -cum RUN/scale.test RUN/CASE/mutex.pprof
go test ./internal/store -run '^$' -bench '^BenchmarkPushReceiptCount$' -benchtime=1s
```

The retained-padding cases distinguish repeated `x` bytes from deterministic
high-entropy strings. PostgreSQL TOAST can compress repeated strings heavily;
that result is not representative of arbitrary retained JSON. `nested18-600k`
adds bounded nested objects and arrays around an 18-output payload.
The original `batch100-100` and `nested18-600k` controls already use 100 and 18
distinct groups per batch: an omitted group count defaults to the output count.
Extend `window` to at least 180 seconds to measure expiry after its one-minute
window has matured.

The receipt microbenchmark seeds only the receipt table. Its timings measure
query cost, not complete ingestion capacity. The cloud soak and its credentials
are independent of this local suite.

## Preserved baseline and next work

The owner deferred the remaining October 14 local runs to prioritize improvements.
The [baseline record](../../docs/development/push-scale-baseline.md) and
[aggregate CSV](results/2026-10-14-baseline.csv) preserve completed measurements
and identify the cancelled case. The
[implementation plan](../../docs/development/push-scale-plan.md) covers Go runtime
changes, an experimental PostgreSQL backend, fair database comparisons and the
matrix to rerun after implementation.

The [implementation results](../../docs/development/push-scale-results.md) record
the candidate/control revisions, resource budgets and qualification limits.
