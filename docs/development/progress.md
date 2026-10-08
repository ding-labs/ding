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
| P05 Compilation, access, packaging | Pending | |
| P06 Watch contract | Pending | |
| P07 Transactional store | Pending | |
| P08 Watch evaluation | Pending | |
| P09 Complete HTTP watch | Pending | |
| P10 Lifecycle, bounds, timers | Pending | |
| P11 Initial adapters | Pending | |
| P12 Inspection and agent use | Pending | |
| P13 Migration and deletion | Pending | |
| P14 Release qualification | Pending | |

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
