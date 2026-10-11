# Live log qualification

October 11, 2026. Live text and JSON capture, counting, and alerting have passed
functional tests and the short load checks below. These results do not establish
sustained production capacity. See the [user guide](../guides/logs.md) for current
behavior and the [implementation plan](log-monitoring-plan.md) for release gates.

## Measured runs

Host: Apple M3, macOS 27.0, arm64, Go 1.26.1. Each run used one stream of 1 KiB
records, alternating five seconds of warnings and five seconds of informational
messages. The collector used a one-second window and one-record minimum, with a
concurrent HTTP watch and real SQLite evaluation and local delivery.

| Format / transport | Duration | Offered records/s | Records counted | Firing latency | Delivery latency |
| --- | --- | --- | --- | --- | --- |
| Text / exec | 15 s | 1,000 | 15,000 | 995 ms | 1,091 ms |
| Text / pipe | 15 s | 1,000 | 15,000 | 992 ms | 1,103 ms |
| Text / file | 15 s | 1,000 | 15,000 | 1,981 ms | 2,079 ms |
| JSON / exec | 15 s | 1,000 | 15,000 | 1,997 ms | 2,092 ms |
| JSON / pipe | 15 s | 1,000 | 15,000 | 1,992 ms | 2,091 ms |
| JSON / file | 15 s | 1,000 | 15,000 | 1,987 ms | 2,082 ms |
| JSON / exec | 60 s | 10,000 | 600,000 | 1,994 ms | 2,092 ms |

Every run matched the independently produced record, warning, and byte counts,
with zero invalid records, analysis drops, or producer writes over 100 ms late.
Firing and recovery events were committed, and the concurrent HTTP watch remained
healthy. The six 15-second runs contain only one measured transition each. The
60-second run contains five transitions after startup; its latency columns are
the nearest-rank p95 of those five samples. This is too few samples to establish
a dependable latency distribution.

Firing latency measures the time from the producer's warning transition to the
successful HTTP acknowledgement of the durable firing. Delivery latency ends at
the fixture's local console sink. Neither includes an external notification
provider. Startup warmup is excluded. Completion of a full second and the
collector's publication phase account for up to roughly two seconds in this
workload; the default 60-second window has different alert behavior.

The [machine-readable summary](evidence/log-qualification-2026-10-11.json) records
exact measurements and source revisions. The initial transport matrix used
`19cb83c`. The final stress run includes the snapshot-boundary, publication-jitter,
and qualification fixes through `bc77cbc`.

## Reproduce a run

From a checkout of this branch:

```sh
go build -o /tmp/ding-log-bench ./cmd/ding-log-bench
/tmp/ding-log-bench --duration 60s --rate 10000 --bytes 1024 \
  --format jsonl --transport exec --out /tmp/ding-log-run
```

The output directory must not already exist. Select `text` or `jsonl`, and
`exec`, `pipe`, or `file`. A separate producer process records its offered load;
the collector cannot silently reduce the offered rate. The harness exercises
the capture adapters, authenticated native summary API, SQLite transaction,
evaluator, and delivery worker. File mode exercises the file follower through
an external native session; it does not measure the daemon file-worker scheduler.
Foreground CLI and daemon reader lifecycle are covered separately by functional
tests.

`report.json` includes counts, latency samples, runtime evidence, and a pass/fail
gate. `producer.json` contains the independently recorded producer counts. The
directory also contains private runtime state; share the reports instead of the
whole directory. Sampled Go heap allocation covers the fixture and runtime
together, not isolated collector RSS or CPU.

## Verification and remaining gates

Functional coverage includes text and JSON classification, explicit level maps,
multiline and oversized records, bounded queue loss, exact window boundaries,
stdout/stderr preservation, exit status, signals, reader shutdown, file rotation
and truncation, session fencing, retries, atomic rollback, readiness, retention,
and separation of daemon readers from polling work. Browser coverage creates,
previews, reviews, applies, inspects, and pauses an external log watch.

`go test -race ./...`, `go vet ./...`, the example manifest validation, Console
unit tests, TypeScript/production build, and JavaScript size budget passed.
The current-channel documentation also passed its strict build.
The full Chromium regression suite passed 23 tests, with two opt-in performance
and visual tests skipped. The new browser flow also passed in WebKit. The installed Firefox
headless binary failed before the test ran with `Could not find profile folder`,
including with a different temporary directory; Firefox verification remains
pending.

Before making sustained capacity claims, run the proposed one-hour baseline,
24-hour soak, default-window workload, multiple-stream sweep, RSS/CPU and storage
measurements, and Linux runtime qualification. The Linux binary cross-build has
passed; that alone does not qualify Linux runtime behavior. Windows capture is
disabled. Eight daemon reader slots are an enforced concurrency limit, not a
claim that eight streams sustain the single-stream stress rate.
