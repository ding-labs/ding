# Legacy hardening verification

Measured October 8, 2026 on an Apple M3 (8 logical CPUs), macOS arm64, Go 1.26.1.
These are local development measurements, not capacity or comparative marketing
claims. Docker Desktop 28.0.4 ran the Linux arm64 packaging checks.

`go test -race -timeout=90s -coverprofile=coverage.out ./...` and `go vet ./...`
pass. Relevant coverage: config 74.6%, transport 96.7%, notifier 86.7%, evaluator
90.4%, ingester 91.3%, server 69.3%. CLI statement coverage is 27.0%; a real
subprocess regression additionally verifies role authentication and SIGTERM
shutdown while inherited stdin stays open. Tests cover the audited defects;
these percentages do not imply every integration or operating system was run.

Both Dockerfiles build. `scripts/container-smoke.sh IMAGE` verifies HTTPS with
bundled public CA roots and rejects the same request with no trust roots. Native
`ding version` and container `ding version` execute successfully. CGO-free builds
pass for Linux/macOS/Windows on amd64 and arm64; Windows execution is not tested
on this host.

A GoReleaser 2.18.2 snapshot produces all six archives, checksums, and a Homebrew
formula with Apache-2.0 and `ding version`. Its standalone `check` reports valid
configuration with deprecated `brews`/`dockers` fields (exit 2). Snapshot release
succeeds. These existing packaging formats are deliberately retained for legacy
Homebrew/Linux compatibility; they are not a clean deprecation check. The release
workflow pins the verified packager version. [GoReleaser's legacy Docker format](https://goreleaser.com/customization/package/docker/).

Benchmark command: `go test ./benchmarks/go -run '^$' -bench . -benchtime=1s -count=1`.
The two window warmups assert 1,000 retained samples before timing; buffers grow
up to their explicit bounds during measurement. Delivery uses a local HTTP test
server, while persistence includes an actual atomic snapshot write and fsync.

| Scenario | ns/op | bytes/op | allocations/op |
| --- | ---: | ---: | ---: |
| One threshold rule | 474 | 368 | 14 |
| One populated window | 7,592 | 1,131 | 24 |
| 100 populated-window rules | 445,016 | 100,989 | 1,806 |
| Engine initialization, 100 rules | 47,667 | 48,331 | 417 |
| JSON parsing | 1,003 | 1,032 | 20 |
| 1,000 distinct groups | 1,413 | 1,046 | 24 |
| 1,000-sample snapshot persistence | 4,968,790 | 101,529 | 1,028 |
| Local HTTP delivery | 26,115 | 7,326 | 80 |

The previous future-dated window benchmark discarded its warmup samples. Its
reported window speed is invalid and the old numbers have been removed from the
current README and landing documentation. BENCHMARKS.md remains explicitly
marked as historical. The old binary-size claim was also withdrawn.

Stripped `-trimpath -ldflags="-s -w -X main.version=0.14.0"` binary sizes:

| Target | Bytes |
| --- | ---: |
| linux/amd64 | 46,489,762 |
| linux/arm64 | 43,450,530 |
| darwin/amd64 | 47,405,760 |
| darwin/arm64 | 44,654,898 |
| windows/amd64 | 47,595,520 |
| windows/arm64 | 43,867,648 |

The Linux arm64 release image measured 43,632,254 bytes locally, including CA roots.

## Published release verification

[v0.14.0](https://github.com/ding-labs/ding/releases/tag/v0.14.0) was published
from `478bc14`; the [release workflow](https://github.com/ding-labs/ding/actions/runs/37852571992)
passed Linux vet/race tests, the container TLS gate, and publishing. All six
downloaded archives match their published checksums. The downloaded macOS arm64
binary prints `ding version 0.14.0`. The published multi-architecture image
`ghcr.io/ding-labs/ding:v0.14.0` passes the positive and negative TLS smoke checks
on Linux arm64. The published Homebrew formula references v0.14.0, Apache-2.0,
and `ding version`. No native Windows runtime result is claimed.

Validation and replay also pass in a read-only container with `--network none`,
a Kubernetes notifier configured, and unwritable alert/state paths. The legacy
maintenance branch is `codex/legacy-maintenance`. A following metadata-only
commit declares the syscall module as direct and gates `go mod tidy -diff`.
