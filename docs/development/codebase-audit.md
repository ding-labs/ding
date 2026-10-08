**Ding product and codebase audit**

October 8, 2026. Recommendation: keep the Go repository and its tested components; build a new watch layer and selectively redesign state, evaluation coordination, and notification delivery. A complete rewrite is not justified by the evidence. The age of the models that produced the code is not a useful quality criterion.

**The product must earn its place beside scheduled agents**

If Ding means “describe something in English and periodically ask an agent to check it,” the differentiation is weak. For a daily airfare check or a weekly game schedule, an existing scheduled agent is a credible substitute, assuming it can access suitable data.

Claude already offers persistent local and cloud scheduling. Its cloud routines also accept API and GitHub triggers. [Claude scheduling](https://code.claude.com/docs/en/scheduled-tasks), [Claude routines](https://code.claude.com/docs/en/routines). OpenAI supports scheduled work with local projects and chat context; supported app events can trigger tasks on eligible web/mobile plans. [OpenAI scheduled tasks](https://learn.chatgpt.com/docs/automations?surface=app).

Nor is “no model call on every check” sufficient differentiation: an agent can write an ordinary deterministic script and schedule that. Such a script can maintain a database and retry webhooks too. Ding would have to save developers from repeatedly implementing and operating that machinery.

The stronger proposed product is an open source runtime for persistent, testable watches that agents can configure and consume. Its value would be explicit time windows, transitions and recovery, durable state and delivery, source adapters, predictable resource use, and evidence explaining each alert. These are proposed requirements, not capabilities the present code already guarantees.

For example: check an API every five seconds; fire after three consecutive failures; notify once until recovery; retain state through restarts; retry a rate-limited destination; expose the triggering observations in a replay. A scheduled agent or generated script could implement that, but a reusable tool should make it a small configuration rather than a new application.

Frequent polling is not a universal solution. Live sports requires a suitable live feed; flight and traffic watches depend on accessible, adequately fresh data. The runtime does not remove provider cost, licensing, availability, or latency constraints. Start with developer-owned HTTP JSON and command output, where access can be made concrete.

**What exists and what it is worth**

The runtime is approximately 5,722 lines of production Go across 40 files under `internal`, with 8,025 test lines across 29 files and 280 named test functions. These counts exclude command entry points, benchmark code, scripts, and website assets. Its size is small enough to understand and refactor incrementally.

The current flow is producer → JSON/Prometheus/jq ingestion → numeric rules and rolling buffers → notification adapters. `ding run` adds subprocess handling, run context, and end-of-run rules; `ding serve` adds HTTP ingestion and optional snapshots. It is a useful event-processing foundation.

It does not yet provide a source scheduler, pull-based watch adapters, natural-language watch compilation, watch lifecycle management, or a durable delivery outbox. The event type is a metric plus a float, labels, and extra numeric fields. Arbitrary documents, entity changes, and event identities need a broader observation model. Numerical rules can remain a supported subset.

| Component | Decision | Reason |
| --- | --- | --- |
| JSON, Prometheus, jq ingestion | Reuse and extend | Useful parsing and transformation with 89.9% package coverage. Preserve compatibility while adding typed observations and explicit source IDs. |
| Numeric condition parser, matching, templates | Reuse with regression fixes | Existing threshold and aggregate behavior is useful. Add transitions, missing-data deadlines, and other temporal operators deliberately. |
| Rolling state, cooldown coordination, snapshots | Substantially refactor | Identity, clock, concurrency, restoration, and retention defects affect the proposed core value. |
| Notification payload formats and provider integrations | Reuse | Useful integration work and tests. Separate payload formatting from delivery mechanics. |
| Notification worker and queue implementations | Replace with shared delivery machinery | Repeated retry workers share failure modes. Add explicit outcomes, durable pending records, retry scheduling, idempotency support, and shutdown behavior. |
| Cobra CLI, `run`, context detection, dry-run rendering, metrics | Reuse selectively | Existing developer workflows and tests are an asset. Introduce new watch commands without forcing them into the old metric configuration. |
| `BuildFromConfig` wiring | Refactor | Validation and replay currently construct runtime notifiers. Separate parsing/validation from starting services and acquiring resources. |
| Watch schema, acquisition adapters, scheduling, agent-facing interface | Build new | This is where much of the new product work lies. Treat natural language as an optional authoring client of a stable schema. |
| Website/docs Workers, release and installer scaffolding | Reuse with cleanup | Small deployment wrappers are straightforward to retain. Marketing claims and packaging require corrections. |

There is meaningful code reuse, but it would be misleading to assign a percentage of the future product before defining its scope. The current code covers much of the processing middle, not most of the proposed watch lifecycle.

**Verified runtime defects**

The baseline suite passes. Additional isolated tests reproduce the following defects; they are saved separately from the normal test suite so the audit does not change production behavior or ordinary test discovery.

1. **High priority: rejected webhooks can be reported as successful.** A local server returning HTTP 429 received one attempt despite a three-attempt configuration, and the success counter increased. `deliver` returns nil for every 4xx; the worker interprets nil as successful delivery. Authentication failures are also misclassified. The same status-handling pattern is present in Slack, Discord, Teams, Telegram, and PagerDuty implementations, though the reproduction exercises the generic webhook. Use explicit delivered/retryable/permanent-failure outcomes and provider-aware retry timing. [webhook.go](../../internal/notifier/webhook.go:169)

2. **High priority: concurrent events bypass cooldowns.** `Process` permits concurrent readers; cooldown checking and setting are separate locked operations. A reproduction produced four alerts within a one-minute cooldown. This is a logical race, so clean race-detector output does not establish correctness. Make the check-and-reserve operation atomic, using the selected evaluation clock. [engine.go](../../internal/evaluator/engine.go:171)

3. **High priority: restored state can override a changed rule window.** Restoring a one-hour rule's state into a same-name one-minute rule preserves the old buffer's window. A ten-minute-old value of 100 and a fresh value of 0 produced an alert with average 50; the new rule should have seen only 0. Persist a rule/state compatibility identifier and migrate or discard incompatible state. This affects reload when persistence is enabled and restart after configuration changes. [state.go](../../internal/evaluator/state.go:74)

4. **Medium priority: distinct label sets share an identity.** `{"a":"x,b=y"}` and `{"a":"x","b":"y"}` both serialize as `a=x,b=y`. They can share cooldowns and rolling samples. Use an unambiguous canonical encoding and migrate existing persisted keys. [engine.go](../../internal/evaluator/engine.go:380)

5. **Medium priority: replay ignores elapsed event time for cooldowns.** The evaluator receives an explicit `now`, and `test-rule` supplies recorded event time, but cooldowns use the wall clock. Replaying two qualifying events two minutes apart under a one-minute cooldown produced only the first alert. Inject one clock through evaluation, cooldowns, and replay. [cooldown.go](../../internal/evaluator/cooldown.go:19), [test_rule.go](../../internal/cli/test_rule.go:121)

6. **Medium priority: window benchmarks do not measure populated windows.** Their evaluation clock is ten minutes ahead of samples in a five-minute window. Reproducing the warmup retained zero samples, rather than the stated 1,000. The measured loop likewise submits expired data. Correct the workload before using these results to support throughput claims. [bench_test.go](../../benchmarks/go/bench_test.go:64)

**Additional operational and packaging findings**

- **Label state is not globally bounded.** Buffer maps, seen-label tracking, and cooldown entries have no general idle-key eviction. A reproduction introducing 1,000 request IDs retained all 1,000 expired samples and 1,001 buffers after processing a new event one simulated day later. The configured buffer limit bounds samples per buffer, not the number of buffers. Seen-label insertion also scans a growing slice. This is particularly unsuitable for a persistent watcher with changing IDs. [engine.go](../../internal/evaluator/engine.go:354)

- **Reload and daemon shutdown can lose queued alerts.** The server stops replaced notifiers, and daemon shutdown calls `Stop` rather than the available `Drain`. Pending deliveries exist only in memory. State snapshots persist cooldowns and buffers, not a delivery outbox; a restored cooldown can therefore suppress a notification that never arrived. The stopping and persistence paths were inspected, not crash-tested end to end. [server.go](../../internal/server/server.go:68), [serve.go](../../internal/cli/serve.go:213)

- **The daemon listens on all interfaces without application authentication.** `/ingest`, `/reload`, `/rules`, and `/metrics` have no authentication middleware. Reachability depends on the deployment network; a reachable caller can inject events and trigger reloads. A developer-local watch tool should default to loopback and require deliberate configuration for remote access. [serve.go](../../internal/cli/serve.go:133), [server.go](../../internal/server/server.go:44)

- **Both scratch images omit CA roots.** The final images copy only the executable. Default Go HTTPS clients on Linux rely on available trust roots; stock images therefore lack the trust store needed for public HTTPS notification endpoints unless operators supply it. This is a packaging finding from static inspection, not a live-container delivery test. [Dockerfile](../../Dockerfile:10), [Dockerfile.release](../../Dockerfile.release:1)

- **The size claim is stale.** A stripped local darwin/arm64 build is 44,704,498 bytes, or 42.63 MiB, versus the README's “5MB” claim. The executable dependency graph includes 259 Kubernetes packages out of 571 total packages; Kubernetes integration is a candidate for a narrower client or optional build. The measurement alone does not attribute all size to Kubernetes. [README.md](../../README.md:316), [kubernetes_event.go](../../internal/notifier/kubernetes_event.go:14)

- **Release checks need cleanup.** The generated Homebrew test calls `ding --version`, which exits 1 with “unknown flag”; the existing command is `ding version`. Its formula declares MIT while the repository license is Apache-2.0. The release workflow runs tests on tags, but there is no ordinary pull-request test workflow in this checkout. [goreleaser configuration](../../.goreleaser.yaml:67), [release workflow](../../.github/workflows/release.yml:23)

**Validation evidence**

| Check | Result |
| --- | --- |
| `go test -race -coverprofile=/tmp/ding-audit-20261008-coverage.out ./...` | Passed; 69.8% total statement coverage |
| `go vet ./...` | Passed |
| Stripped local binary build | Passed on Go 1.26.1, darwin/arm64 |
| Additional audit reproductions with race detector | Six expected failing tests expose defects; one passing observation records retained stale state |
| Homebrew version command | Failed as configured; `ding version` succeeds |

Package coverage includes evaluator 84.8%, ingester 89.9%, notifier 79.1%, server 63.3%, config 68.6%, and CLI 23.9%. Run context is 91.4%; dry-run formatting and metrics are 100%. Coverage is useful evidence of existing investment, not proof that temporal or delivery semantics are correct.

Source inspection covered the Go runtime, tests, benchmark setup, configuration and release paths, installer, and small website/docs/install Workers. This does not establish live provider compatibility, production load capacity, cloud deployment correctness, or comprehensive vulnerability coverage. Production code and pre-existing worktree changes were not altered.

The [reproduction source](../../testdata/audit/reproduction_test.go.txt) and [recorded output](../../testdata/audit/reproduction.log) are retained beside this report. Run the isolated tests from the repository with a Go overlay:

```sh
python3 - <<'PY'
import json
from pathlib import Path
root = Path.cwd()
source = root / 'testdata/audit/reproduction_test.go.txt'
overlay = {'Replace': {str(root / 'internal/evaluator/zz_audit_test.go'): str(source)}}
Path('/tmp/ding-audit-overlay.json').write_text(json.dumps(overlay))
PY
go test -race -overlay=/tmp/ding-audit-overlay.json ./internal/evaluator -run '^TestAudit' -count=1 -v
```

The concurrent reproduction is a stress test and its observed duplicate count can vary. The other five failing reproductions are deterministic. These tests are expected to fail until their respective defects are fixed.

**Recommended implementation sequence**

1. Preserve the current CLI and numeric rule behavior with the existing suite. Turn the audit reproductions into regular regression tests as each defect is fixed. Correct packaging and add tests on pull requests.
2. Establish one explicit clock, collision-safe identities, atomic transitions, bounded retention, and versioned persistent state. Consolidate notifier delivery and define retry and crash recovery behavior. Aim for durable at-least-once delivery with stable event IDs; do not promise universally exactly-once notifications.
3. Introduce a separate watch schema and runtime with HTTP JSON and command sources. Add source cursors, schedule state, structured failure reporting, and an event history. Keep current metric ingestion as a supported adapter.
4. Demonstrate the five-second API watch end to end, including restart, recovery, and rate-limit cases. Add natural-language creation through an agent only after the generated configuration can be inspected and replayed.
5. Validate that developers prefer this to maintaining generated scripts before expanding to flights, traffic, sports, or a large provider catalog. A hosted service can later sell operation of these same watches, shared administration, history, and delivery reliability; monetization is a hypothesis to test, not evidence of demand.

The investment decision is therefore to retain the repository, interfaces worth preserving, and tests; concentrate new engineering on the semantics and operational behavior that would make Ding worth adopting beside an agent.
