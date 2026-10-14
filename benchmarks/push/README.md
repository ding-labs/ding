# Local push stress tests

This suite measures Ding's authenticated JSON ingestion through real local TLS,
workspace SQLite stores, evaluation, receipts, and optional webhook delivery.
Every offered key is reconciled against durable receipts, usage, and observation
rows after measurement. Profiles exclude setup, warmup, and reconciliation.

## Run cases

Run from the repository root with Go, Python 3, and Docker for container cases:

```sh
python3 benchmarks/push/run.py --environment native --cases generator-2500,one-100,one-500 --seconds 30
python3 benchmarks/push/run.py --environment container --cases generator-2500,one-100,one-500 --seconds 30
python3 benchmarks/push/run.py --environment native --cases one-250,one-500 --repeat 3
python3 benchmarks/push/run.py --environment container --cases sustain-many
```

`cases.json` contains payload, batch, group, receipt-aging, rolling-window,
concurrent-read, noisy-workspace, destination, and restart scenarios. Screen
briefly, then repeat boundary cases for three minutes and stable cases for thirty
minutes. Mature rolling-window measurements need more than one minute.

The producer runs as a separate host process. Only the container server gets the
1 CPU and 1 GiB RAM limits. Its databases use a Docker local volume; evidence and
the producer ledger use the host filesystem. The volume has no 1 GiB size cap;
compare recorded growth with Railway's actual storage allocation separately.
Native and container filesystem performance cannot establish Railway capacity.

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
no read errors, and a drained outbox. These are local test targets. Restart is a
graceful store/worker reopen, not a power-loss or physical process-crash test.
Outbox drain is bounded to one minute; remaining work is reported explicitly.

```sh
go tool pprof -top -cum RUN/scale.test RUN/CASE/cpu.pprof
go tool pprof -top -cum RUN/scale.test RUN/CASE/mutex.pprof
go test ./internal/store -run '^$' -bench '^BenchmarkPushReceiptCount$' -benchtime=1s
```

The receipt microbenchmark seeds only the receipt table. Its timings measure
query cost, not complete ingestion capacity. The cloud soak and its credentials
are independent of this local suite.
