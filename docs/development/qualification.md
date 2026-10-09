# Watch release qualification

P14 is in progress. A real 24-hour soak and all six native artifact jobs must
finish before the stable gate can be marked complete. Short fixture runs are
harness checks, not substitutes for elapsed soak time.

## Fault matrix

| Fault | Executable evidence |
| --- | --- |
| Transaction crash/commit boundary | `store.TestCrashTransactionBoundaries` subprocess termination |
| Physical disk full | `scripts/filesystem-faults.sh`: isolated 8 MiB tmpfs filled to ENOSPC; rollback, 503, same-key retry after freeing space |
| Read-only filesystem | Same script: real read-only mount refuses startup, committed observations/events/outbox and integrity preserved |
| Migration failure and backup restore | `store.TestBackupAndFailedMigration`; native executable qualification restores a verified backup into a fresh directory |
| Single writer | Store lock tests, including subprocess and canonical/symlink paths |
| Ambiguous remote receipt | `watchrun.TestReceiverAcceptsThenProcessDiesBeforeAcknowledgment`: kill after receipt, expire lease, retry same event ID |
| Credential rotation | `watchrun.TestCredentialRotationUsesPinnedReferenceOnManualRetry`: resolve updated secret with immutable destination revision |
| Rate limit/order/slow destination | Delivery provider tests, durable destination backoff tests, five-second restart demonstration |
| Blocked console | `watchrun.TestBlockedConsoleHasOneOutstandingWriteAndBoundedShutdown`: one outstanding write, bounded worker shutdown, retained delivery |
| Process-tree cancellation | Native command adapter tests on Linux, macOS and Windows |
| Apply/ingest/shutdown races | Lifecycle and control tests under the race detector where supported |
| Wall-clock discontinuity | Forward/backward tests distinguish monotonic scheduler delay from wall-time jumps, fence acquisitions, replay unknown state and rearm deadlines |

Clock detection compares wall and monotonic elapsed time while the runtime is
running, with a two-second tolerance. It cannot reconstruct monotonic elapsed
time across process restarts. A jump preserves open incidents, resets streaks,
rearms missing-data deadlines, and requires a fresh full numeric window. Active
dedup IDs and cooldowns conservatively receive their full duration again.

A generic embedded `io.Writer` cannot be forcibly canceled. Console delivery
allows at most one outstanding write per App; workers stop on cancellation, but
that single goroutine needs the writer to unblock/close. The standalone process
exits normally. A late write can duplicate remote receipt. Filesystem syscalls
that never return are outside the ordinary five-second delivery drain guarantee.

## Native artifact gate

CI executes adapter/persistence/lifecycle contracts and an actual stripped
executable on Linux amd64/arm64, macOS amd64/arm64, Windows amd64/arm64. It checks
the executable's reported OS/architecture against the runner, then performs
fresh daemon startup, validation, dry-run/apply, authenticated push, event
inspection/replay, forced restart, export, verified backup and offline restore.
Go 1.26 does not support the race detector on Windows ARM64; that runner executes
the same contracts without `-race`. Other native runners use it.

Run a local artifact drill with:

```sh
go build -trimpath -ldflags='-s -w' -o /tmp/ding-native ./cmd/ding
DING_BINARY=/tmp/ding-native go test -v -count=1 ./internal/qualification
```

The first local Darwin ARM64 drill passed and measured 14,715,922 executable
bytes. This is a development build measurement, not a published release size.

CI [37867026684](https://github.com/ding-labs/ding/actions/runs/37867026684)
passed all six native targets and the physical filesystem faults on commit
`a94a47b`. Later runtime changes must pass the same jobs again.

## Capacity fixture

`cmd/ding-soak` runs 100 real local HTTP sources at five-second intervals, one
steady push per second outside bursts, and a 30-second burst of 200 individual
pushed observations per second each hour. Every accepted push must fire and
deliver exactly one console event to a counting sink. Startup waits for each HTTP
watch's first committed poll before the capacity clock begins; simultaneous
startup can return retryable busy responses.

The producer holds at most 256 outstanding requests and retries a busy admission
every 5 ms for at most one second. The report counts those retries separately
from lost/rejected inputs. This models a producer honoring backpressure; it does
not promise every first HTTP attempt is accepted. Its push calls use the same
production projection/acceptance path directly, avoiding source/destination
network time. Measured latency includes projection, admission retries, database
contention, evaluation and durable commit. Admission/auth HTTP behavior is tested
separately. The driver uses absolute arrival times and fails if more than 1% are
over 25 ms late, so a slow producer cannot silently lower the offered load.

Linux `/proc/self/status` supplies current RSS and the high-water mark. The
memory budget includes the embedded driver and fixture HTTP server. Growth
compares median RSS after the 15-minute warmup with the final ten-minute window;
the peak gate uses the kernel high-water mark, not just periodic samples. Latency
uses bounded one-millisecond histogram buckets (the reported p95 is an upper
bound). Counts, live database pages, retained rows and database/WAL bytes are
sampled each minute. A verified backup and integrity check finish the run.

The planned machine is an Apple M3 host with 16 GiB RAM, running Docker Linux
ARM64 with a **four-CPU quota** and 1 GiB memory ceiling. The target remains below
250 MiB process RSS; the container ceiling does not relax it. Data resides on a
Docker volume, not a shared host filesystem. Ordinary history is explicitly
**one hour** for this fixture, so cleanup can be observed. Input receipts retain
their normal 24-hour dedup horizon. The default seven-day history setting is not
a claim that this workload fits seven days inside the default 1 GiB store.
Report actual growth and capacity implications before recommending defaults.

From a clean, committed checkout:

```sh
scripts/start-soak.sh
```

It starts a detached container and prints its exact name and collection commands.
Keep the host and Docker awake for the full run. No external provider or alert
recipient is contacted. `progress.json` is a checkpoint, not a passed result.
Only `result.json` with `status: passed`, 86,400 seconds of requested/elapsed time,
zero final failures and a successful container exit satisfies the soak gate.
Retain the initial failed trials as diagnostic evidence; they are not releases.

The first trial found two real defects: a queue predecessor lookup scanned
delivered history, and concurrently acquired timestamps could commit out of order
and create false clock reversals. Schema 3 adds live-queue/retention indexes, and
live ingestion now samples accepted time inside the transaction. Regression
tests cover query plans, migration backups and concurrent firing counts.

## Measured packaging baseline

GoReleaser 2.18.2, Go 1.26.1, CGO disabled, `-s -w`, snapshot of runtime
commit `68d52a3`. These are exact bytes; archive metadata/version strings can
change sizes slightly between releases. Adopt an initial review budget of
18 MiB per stripped executable and 8 MiB per compressed archive; investigate
increases rather than repeating the old unsupported 5 MB claim.

| Target | Executable bytes | Archive bytes |
| --- | ---: | ---: |
| Linux amd64 | 15,524,002 | 6,467,480 |
| Linux arm64 | 14,680,226 | 5,969,956 |
| macOS amd64 | 15,581,680 | 6,446,255 |
| macOS arm64 | 14,748,962 | 6,079,082 |
| Windows amd64 | 15,728,640 | 6,519,050 |
| Windows arm64 | 14,579,200 | 5,924,773 |

All six archives include the example, README, license and checksums. The native
macOS ARM64 archive passes the exact README manifest, one firing after repeated
high samples, recovery, event replay, forced restart and backup restore. The
Linux ARM64 release container passes real HTTPS polling with CA verification and
a negative control without CA roots. No snapshot is published as a stable release.
The release container measures 14,861,950 uncompressed image bytes; use a 20 MiB
image review budget for this baseline. Registry transfer sizes are different.

CI [37868321420](https://github.com/ding-labs/ding/actions/runs/37868321420) passes
the schema-3 runtime (`68d52a3`), including all six native targets, full race tests,
vet, module consistency, containers and physical filesystem faults. The local
integrated race profile reports 81.2% overall statement coverage; fault assertions
and subprocess outcomes supplement coverage percentages.

## Completed short harness check

The six-minute preflight passed on 2026-10-09 UTC: 18,270 accepted/delivered
pushes, zero lost/rejected inputs, 12 ms steady p95, 6 ms burst p95, 29.2 MiB
peak RSS and -0.8% post-warmup median RSS growth. This short run uses a two-minute
warmup, one-minute history and a burst every two minutes to exercise cleanup
quickly. It is not the 24-hour gate or a published capacity guarantee.

The [raw six-minute result](https://github.com/ding-labs/ding/blob/codex/ding-watch-runtime/testdata/qualification/harness-6m.json)
retains the original trial revision label; the runtime changes were committed as
`68d52a3`. The driver is committed with this report. Failed early trials exposed
the defects above and producer admission limits; the final driver records bounded
admission retries and does not discard accepted events.
