# Watch runtime implementation progress

The implementation follows `watch-migration.md` sequentially. A step is marked
complete only after its acceptance checks pass. Commits record completed steps;
failures and environment limitations are recorded here without implying success.

| Step | Status | Evidence |
| --- | --- | --- |
| P01 Baseline and CI | Complete | Race suite passes, 69.8% coverage; vet and actionlint pass; CGO-free builds pass on Linux/macOS/Windows amd64/arm64. Audit fixtures and plan retained. |
| P02 Time and identity | Complete | Full race suite and vet pass; identity 100%, evaluator 89.7% coverage; key and label fuzzing pass. Cooldowns use explicit time and atomic reservation. Snapshot encoding version bumped to reject ambiguous old keys. |
| P03 State and limits | Complete | Full race suite, affected-package rerun, and vet pass. Evaluator 90.4%, server 65.3% coverage. Tests cover changed/removed rules, malformed/oversized state, active-state retention, idle pruning, atomic failed restore, HTTP quota rejection, and explicit startup failure. |
| P04 Delivery and lifecycle | Complete | Full race suite and vet pass. Transport 96.7%, notifier 86.7%, server 68.9% coverage. Provider rejection/rate limits, retry deadlines, queue capacity, concurrent send/drain, and ingest/swap/close regression tests pass. |
| P05 Compilation, access, packaging | Complete | Full race suite, vet, actionlint, six target builds, native daemon auth/shutdown subprocess regression, and both container TLS smoke tests pass. GoReleaser snapshot packages all archives and corrected Homebrew metadata. Published v0.14.0; release CI, archive checksums, native version, published container TLS, and Homebrew metadata verified. See legacy-hardening.md. |
| P06 Watch contract | Complete | Full race suite plus affected-package rerun, vet, module consistency, six target builds, and actionlint pass. Compiler 83.0%, watch 100%, CLI 95.3% coverage; ~148k manifest fuzz executions pass. Independent JSON Schema validation passes for examples and normalized output. |
| P07 Transactional store | Complete | Store 83.1% coverage; full race suite, vet, module consistency, and actionlint pass. SQLite driver test binaries build for all six targets. Full store suite passes in Linux arm64 container, including subprocess crash/lock tests, backup/restore, leases, rollback, and SQLite write-failure cases. |
| P08 Watch evaluation | Complete | Full race suite, vet, and module consistency pass. Condition coverage 94.2%, replay 93.2%, CLI 97.4%; 75,573 deterministic-evaluation fuzz cases pass. Offline example replay fires once and recovers once. New CLI dependency graph contains no legacy evaluator/notifier or Kubernetes packages. |
| P09 Complete HTTP watch | Complete | Full race suite, vet, module consistency, and six CGO-free CLI builds pass. Actual five-second subprocess/restart/429 demonstration passes. HTTP source 91.2%, control API 90.7%, CLI 90.3%; isolated runtime unit suite 83.2% (full subprocess-including coverage reports 57.8%). Atomic rollback/dedup, lease fencing, delivery order, auth, bounded polling, and graceful shutdown verified. |
| P10 Lifecycle, bounds, timers | Complete | Full race suite with cross-package coverage, affected-package rerun, vet, module consistency, actionlint, and six builds pass. Integrated coverage: store 82.1%, condition 93.9%, runtime 82.5%, control 87.4%, CLI 91.0%. Revision/reset/cancel races, durable timers and replay, quota rollback/gap recovery, dedup expiry, and evidence-safe retention pass. |
| P11 Initial adapters | Complete | Full race suite, vet, six target builds, and native Linux/macOS/Windows CI pass. Command process-tree cancellation, bounded shared jq, authenticated transactional push, durable signal baselines/dedup, Slack/Discord outcomes, and destination revision backoff verified. CI run 37861926592; follow-up backoff regression and full race suite pass. |
| P12 Inspection and agent use | Complete | Full race suite, vet, module consistency, actionlint and six target builds pass. Schema-1 upgrade/backup, scoped cursor expiry, bounded inspection, retained checkpoint replay, secret-safe export/doctor, retry policy cycles, and complete authenticated CLI authoring workflow verified. Skill validator passes. |
| P13 Migration and deletion | Complete | Live legacy parity captured in 74801ba before deletion; 17 retained fixtures cover supported behavior and the declared window-boundary change. Default CLI cutover, pure converter/report, removed-command guidance, dependency audit, full race suite/vet, six GoReleaser archive builds, both container TLS smokes, pinned legacy installer, and strict docs build pass. Legacy runtime/Kubernetes graph removed. |
| P14 Release qualification | In progress | Physical filesystem/crash/clock faults pass; schema-3 queue and concurrent acceptance fixes pass full race tests. CI 37868321420 passes all six native artifacts and containers. Six archives, exact quickstart, backup/restore, size report and strict docs build pass. Real 24-hour soak remains a required gate; no stable qualification claim yet. |

The working branch is `codex/ding-watch-runtime` in an isolated checkout.
Pre-existing website/documentation edits in the original checkout are preserved.

P04 consolidates all seven asynchronous legacy adapters on one bounded worker.
HTTP attempts reject redirects, distinguish permanent/retryable outcomes, honor
Retry-After, and validate Slack/Telegram/PagerDuty acknowledgments. Shutdown
rejects new sends, drains within its deadline, cancels requests, and reports
unfinished work. Reload copies live compatible state only after old dispatches
finish; parsing/transform changes invalidate state. HTTP and SIGHUP share this
path. The legacy queue is still memory-only: process crashes or drain deadlines
can lose unfinished delivery. P07–P09 introduce the durable replacement.

P10 makes CI account for integration tests with `-coverpkg=./...`. It also captures
subprocess test output so a child coverage line cannot replace the parent
package's displayed percentage. Earlier P09 isolated/full percentage differences
were reporting artifacts; the P10 merged profile measures exercised statements
across the complete suite.
