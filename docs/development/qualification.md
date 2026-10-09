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
