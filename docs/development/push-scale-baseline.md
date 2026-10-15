# Local push performance baseline

Recorded October 14, 2026, Pacific time. The owner deferred remaining runs to
prioritize improvements. All local load processes are stopped. Completed results
and the cancelled case's database archive are preserved privately. The independent
Cloud push soak continues.

The [aggregate CSV](../../benchmarks/push/results/2026-10-14-baseline.csv) contains
88 completed cases and one cancelled case across seven run directories. Raw
request ledgers, profiles, logs, databases and fixture credentials remain outside
Git. Harness revisions vary by run; the measured application behavior is based
on `f8cd4f8b8d55f792f2e3674c68100acec1eea137` throughout these runs.

## Three minute controls with profiling off

These cases use a separate host generator, 512 workers, synthetic identities,
one small observation per request, real authenticated TLS ingestion and raised
test-only admission limits. The small body is 55 bytes. SQLite retains WAL and
FULL synchronous durability. Every completed case in this table reconciled its
acknowledged receipts, usage and observation counts and passed replay/conflict
and final-state checks.

| Server and workload | Repeats | Accepted requests per second | Acceptance p99 | Repeats meeting all local targets |
| --- | --- | --- | --- | --- |
| Native, one workspace, 250 planned/s | 3 | 222–250 | 24–2,341 ms | 1 |
| Native, one workspace, 500 planned/s | 3 | 399–451 | 1,264–1,865 ms | 0 |
| Native, ten workspaces, 500 planned/s total | 3 | 488–500 | 90–1,573 ms | 1 |
| Native, ten workspaces, 1,000 planned/s total | 3 | 991–996 | 215–568 ms | 1 |
| Linux server, 1 CPU and 1 GiB, one workspace, 250 planned/s | 3 | 194–247 | 1,374–6,381 ms | 0 |

Acceptance latency includes delay from the planned arrival, queueing and producer
ledger work. A passing repeat does not establish a consistently sustainable rate.
The native machine is an Apple M3 with 8 cores and 16 GiB RAM, shared with ordinary
desktop work. Docker uses a local volume without Railway's 1 GiB storage cap.
These measurements establish improvement targets, not Railway capacity.

## Other useful evidence

- The unprofiled three-minute slow-receiver case accepted all 4,500 requests at
  25/s, with p99 74 ms, but left 2,338 pending deliveries after a one-minute drain.
  The receiver delays each request 100 ms and shares the server's resources.
- The unprofiled three-minute outage/recovery case accepted all 1,800 requests at
  10/s, with p99 29 ms, recovered its group and drained its outbox.
- CPU, blocking and mutex profiles identify credential connection contention,
  workspace serialization and full group-state loading. The
  [improvement targets](push-scale-improvement-targets.md) record their scope.
- The receipt-count microbenchmark grows from roughly 7 microseconds with no
  receipts to 1.23–1.26 ms with 100,000. It measures the query and read transaction,
  rather than full ingestion.

## Evidence qualifications

The native aged-store run reached 100,000 accepted preload requests and 100,500
before measurement. The laptop then entered clamshell sleep for 27 seconds. Ding
recorded a clock discontinuity, advanced the watch generation and rejected 35
in-flight requests with `inactive_source`. All 17,965 acknowledged measured inputs
matched durable push usage. One internal clock observation caused the strict
all-observation count check to fail. Preserve this run as clock recovery evidence;
repeat it on an awake host before using it as an aged-store capacity control.

Earlier diagnostic cases had profiling enabled and some used older harness
behavior. The first native 10,000-group screen populated only 6,013 groups, and an
earlier aged-store case started below 100,000 receipts. Their configured names do
not establish the actual population. CSV `bodyBytes` is the configured target;
zero means the generated small body. Missing historical verification fields stay
blank. Historical `stable` values retain each run's original classification.

The cancelled container `one-500-1` has no measurement report. Its unknown counts
remain blank, its database is archived privately, and it contributes no capacity
claim. Large-payload ramps and four thirty-minute runs are deferred. Reopening
stores in completed cases was graceful; crash and physical power-loss capacity
qualification remain separate tests.
