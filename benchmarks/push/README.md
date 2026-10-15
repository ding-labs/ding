# Local push stress tests

This suite measures Ding's authenticated JSON ingestion through real local TLS,
SQLite or PostgreSQL workspace stores, evaluation, receipts, and optional webhook delivery.
Every offered key is reconciled against durable receipts, usage, and observation
rows after measurement. Capacity runs leave profiling off. Use `--profiles` for
diagnostic runs; their profiles exclude setup, warmup, and reconciliation. Compare
profiled results with unprofiled controls before making a capacity claim.
With PostgreSQL, `--profiles` also enables `pg_stat_statements` in the disposable
fixture. Resource snapshots contain cumulative app-role planning/execution, block
and WAL counters grouped by bounded operation names; subtract snapshots for a
measurement interval. SQL text, parameters and credential roles are not exported.
This profiling overhead is excluded from ordinary capacity runs.
Use the report's `StartedAt`/`EndedAt` and each database counter's timestamp to
select the measured interval: launcher samples also cover setup and reconciliation.
`databaseEnd` is after fixture teardown, so a dropped test database can reduce its
database-wide totals; it is not a final retained-storage measurement.

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

The fixed-arrival driver persists a compact plan of every possible request key
before starting the measurement clock (at most one million arrivals per run).
Responses use a bounded 4,096-entry background journal that must flush successfully
before the run completes. A crash can lose response records, but every planned key
can still be resolved against durable receipts. Journal backpressure still counts
as generator lateness; slow storage is never hidden or treated as a passing run.

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

`business-mixed` and `business-mixed100` use a declared synthetic mix across
10/100 workspaces: 80% 10 KB/one-output webhooks, 15% 100 KB/18-output batches,
and 5% 600 KB/100-output batches. Bodies include nested workflow/job metadata and
deterministic high-entropy padding; padding is parsed but not retained. This is a
representative shape test, not a captured customer trace. Payload weights are
explicit, and the report records per-class accepted counts, bytes, observation
counts and p99. Each 20 arrivals represents 1.06 MB and 170 observations. Reads
and graceful restart reconciliation are enabled. Run retained-padding, alert
and HTTP-polling scenarios separately before claiming a complete business mix.
The default weighted cycle groups arrivals into short producer/workspace blocks.
For an interleaved Railway control, pass `--seed 42` to `railway_collect.py` (or
`railway_run.py` inside the generator). The seed shuffles the complete weighted
cycle reproducibly, preserving its exact counts and every planned receipt key.
Keep burst and interleaved results separate; they have different queueing behavior.

## Interpret evidence

### Railway private-network lab

`Railway.Dockerfile` builds the same Go test server and load driver into an
isolated image. Deploy separate app (`railway_app.py`) and generator
(`railway_run.py`, with no arguments) services beside a dedicated PostgreSQL
service. Pin both source deployments to the same commit. No public domain or
database proxy is required. Business logic remains in Go; Python starts cases
and collects evidence. This is the real ingestion handler and storage stack in
a test process, with expanded lab admission limits, rather than a complete
production deployment or a default-policy capacity claim.

Both services require a shared private `DING_LAB_CONTROL` secret and
`DING_TEST_RAILWAY_PROJECT` equal to Railway's injected `RAILWAY_PROJECT_ID`.
Set `DING_SCALE_PRIVATE_HOST=ding-scale-app.railway.internal`. The app additionally
requires `PGADMIN_URL` referencing the dedicated Postgres service,
`DING_LAB_PG_CA` containing its issuing CA certificate, and `DING_SCALE_TLS_CERT` /
`DING_SCALE_TLS_KEY` under `/data/ding-lab`. PostgreSQL uses verified TLS with
fsync, synchronous commits and full-page writes enabled. The launcher creates
only random `dingtest_*` databases and verifies ownership of its two test roles;
successful fixtures drop their database, while failures retain evidence for
inspection. Never use this launcher against the production soak database.

From SSH on the generator, run one case at a time:

```sh
python3 /opt/lab/railway_run.py --case generator-mixed --rate 100 --seconds 30
python3 /opt/lab/railway_run.py --case business-mixed --rate 25 --seconds 180 --pool 4
```

Copy private run artifacts before redeploying either ephemeral service. Record
actual cgroup limits, pinned revision, database settings and resource samples
for every case. Resource observations include setup and reconciliation; select
only the report's measurement timestamps when calculating CPU and I/O rates.
Calibrate the generator at or above each intended rate before accepting a
capacity result. Railway qualification requires every planned first attempt
accepted, scheduling p99 at most 20 ms and acceptance p99 below 250 ms, plus
reconciliation, reads, restart and resource-observation gates.

Start with Ding at 1 CPU/1 GB and PostgreSQL at 1 CPU/2 GB. Independently test
PostgreSQL at 2 CPU/4 GB and 4 CPU/8 GB, holding Ding fixed; then Ding at 2 CPU/2 GB
and 4 CPU/4 GB, holding PostgreSQL fixed. Test the best combined configuration
and repeat a previous size as a control. Report throughput per allocated CPU and
memory as well as absolute throughput. Keep database settings constant for the
resource-only comparison; label any tuning separately. Connection-pool sweeps
(2/4/8/16) are separate experiments, not silent changes between sizes.

Screen rates briefly, repeat promising boundaries three times for 180 seconds,
then sustain an operating rate with headroom for 30 minutes. Include 10/100
workspace mixes, retained high-entropy JSON and alert delivery controls. This
does not establish multi-server failover, physical crash recovery or arbitrary
customer workload capacity. Export evidence, remove only owned fixture
databases/roles and temporary services, and restore the dedicated database's
prior resource allocation when the matrix ends. Preserve its existing volume.

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
