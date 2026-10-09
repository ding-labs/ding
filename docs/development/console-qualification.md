# Console qualification

Recorded October 9, 2026. This is an implementation/qualification record, not a production release approval. The console is developed in `codex/ding-console`; the primary checkout's website deployment drafts and the existing soak container/volume remain separate.

## Implemented surface

All runnable CLI commands have an enforced entry in `testdata/console/parity.json`. Watches, event-time evidence, delivery attempts/retry, validation/explanation/simulation, whole-bundle review/apply, lifecycle operations, export, replay, Doctor, destinations, backup, migration, version/help and completion have usable console paths. Process startup, shutdown and startup configuration are explicit host setup boundaries. There is no browser terminal or ingestion credential exposure.

The console uses the actual Go compiler/evaluator and SQLite store. The four-stage evidence view follows observed input → evaluation → recorded event → delivery. Replay result/window aggregates come from the Go evaluator; verification happens after releasing the database read transaction. Definitions, payloads and large JSON previews load only when selected.

## Checks executed

[CI run 38001968528](https://github.com/ding-labs/ding/actions/runs/38001968528) passed all jobs for implementation commit `f314046`: race/coverage, console/browser tests, six cross-builds, six native artifact platforms, and source/release container qualification. Subsequent qualification-record edits are documentation only.

Reference host: Apple M3, macOS ARM64, Go 1.26, Node 24.15. Browser tests start actual console-enabled binaries with separate temporary stores. Providers and receivers are local deterministic fixtures.

| Check | Result |
| --- | --- |
| `go vet ./...` and `go test -race -tags console ./...` | Passed across all packages, including generated contracts and full CLI inventory |
| Component tests | 6 passed: event semantics, timestamps/missing values, retention gaps, large untrusted JSON, review diff |
| Browser journeys | **51 passed** in CI: 17 critical journeys each in Chromium, Firefox and WebKit. Six opt-in screenshot/performance cases are intentionally skipped in the ordinary suite. Affected local journeys were rerun after the focus fix. |
| Firefox portability | The full suite passed on Linux in CI and again locally in the isolated Linux ARM64 container: 17 passed, two opt-in cases skipped. Local macOS Firefox could not launch; the container avoids changing personal browser permissions. |
| Accessibility | No serious/critical axe findings on evidence at 375/768/1024/1440 px in both themes, System, and Workbench including the syntax editor. Keyboard command navigation, dialog behavior, focus return, and reauthentication draft recovery exercised in browser tests. |
| Native installed artifact | Passed fresh install, embedded deep link/assets, apply, authenticated push, event replay, forced restart, verified backup and offline restore on darwin/arm64 |
| Browser downloads | Export validates with `ding validate`; evidence verifies with `ding replay`; downloaded SQLite backup starts a separate healthy daemon containing the applied watch |
| Console platforms | Cross-compilation passed locally for linux/darwin/windows × amd64/arm64. **All six native artifact jobs passed in CI**, including real source, persistence, lifecycle, embedded asset and backup/restore checks. |
| Source/release containers and filesystem faults | **Passed in CI**: trusted/absent-CA controls, embedded deep links/assets, and physical ENOSPC/EROFS rollback/recovery. Final source/release image smoke tests also passed locally on Linux ARM64 after Docker resumed. |
| Authentication/recovery | One-use/expired links, exact origin/Host, CSRF, ingest isolation, logout, daemon-scoped cookies, backup ownership/expiry/quota, revision conflicts and unknown mutation outcomes covered |

Critical browser journeys include HTTP, command and push creation/simulation; atomic review/apply; destination conflicts; incident evidence/delivery; terminal retry; pause/resume/delete; stale connection behavior; live event following; Doctor/help/completion; legacy conversion without automatic apply; session recovery; and lost responses after a real committed mutation. No automatic mutation retry is permitted.

## Performance evidence

- First usable 1,000-watch list with warm assets, Chromium on the reference host: **67 / 59 / 57 / 50 / 60 ms** across five navigations (1-second budget). This browser fixture has 1,000 definitions; the separate SQLite fixture below exercises retained history/entities and concurrent work.
- Initial JavaScript: **145.8 KiB gzip**, below the 250 KiB budget. Optional CodeMirror code is a separate lazy chunk (~104 KiB gzip); the accessible plain editor works without it.
- SQLite fixture: **1,000 watches, 100,000 retained events, 20,000 entity checkpoints**. Thirty paired watch/event reads measured **111 ms p50 / 206 ms p95**, while 16 new inputs were accepted and delivered concurrently. The assertion requires p95 below one second and at least ten accepted inputs. CI independently measured 171 ms p50 / 174 ms p95 with 15 live accepts/deliveries. These are read-contention measurements, not a continuous-load SLA.
- Ten short `ps` samples per mode on the reference host, separate fresh stores: headless idle median **25,096 KiB** (peak 25,504); console idle **24,656 KiB** (peak 24,960); active console API reads **26,720 KiB** (peak 27,424). Small differences are allocator/process noise; this is not a memory-growth soak. Active mode read status/watches/events/deliveries every 300 ms on an empty store.
- Lists default to 50 rows, capped at 100. Live event follow polls every three seconds, list snapshots every five, instance status every fifteen. Nonessential background polling is disabled. Tool, proof, Doctor and backup concurrency are independently bounded.

Reproduce the data-scale check with:

```sh
DING_CONSOLE_SCALE=1 go test ./internal/watchrun -run TestConsoleScaleQualification -count=1 -v
```

An opt-in first-usable-list browser measurement is available with `DING_CONSOLE_PERF=1 npx playwright test --project=chromium tests/performance.spec.ts` inside `web/console`. It applies 1,000 watches in permitted batches and measures five warm-asset navigations, enforcing a one-second local budget. Keep this reference-hardware measurement separate from noisy shared CI timings.

## Visual review

Reviewed actual daemon screenshots for the attention list, Workbench, watch detail in both themes, a 375 px mobile detail and the complete event evidence panel. Files in `docs/images/console` are isolated preview fixtures, not customer data. The review led to mobile header/instance improvements, readable operators, pluralization, replayed results, retained filter/focus context and dark-theme editor colors.

## Remaining release gates

- Human screen-reader review and short task sessions with developers who did not build the console. Automated accessibility and keyboard tests do not substitute for these. The 30-second evidence/delivery comprehension goal remains unmeasured.
- A new continuous release soak against the console-enabled candidate. The earlier container `ding-soak-3db426f-20261009205334` exited at 22:03:15 UTC with a clock discontinuity (~3m30s difference), incomplete elapsed time/polling and insufficient post-warmup samples. It is preserved and does not qualify this changed binary. No 24-hour result is claimed.

U07's implementation and automated qualification are complete; release qualification remains open until the human review and continuous soak are complete. Do not merge or publish solely because the UI looks complete.
