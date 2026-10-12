# Cloud push qualification

October 12, 2026. Implementation branch: `codex/cloud-push`, based on `main` at `34ad00a659dc4b1e0532a7b5a653edc33bf9e93c`. This is a source implementation and local qualification record. **It does not declare the public push service production-qualified.**

## Deployment state

Railway service `ding-cloud` now follows `ding-labs/ding:main` (not a pinned commit). Deployment `41073232-d0f1-400c-8b9a-af2ba3d1ad9f` succeeded at `34ad00a`; `https://cloud.ding.ing/readyz` returned 200 with ready status. That deployment precedes this push implementation. The existing allocation is one replica, 1 CPU / 1 GiB memory, a 1 GiB persistent volume at `/data`, and the public domain `cloud.ding.ing`.

PR #42 merged into `main` at `5f2d51293e6acf5a3ed876ea0dacc271ccfb6a85`. See the live qualification update below. Native adapters default to disabled. Schema changes require a coordinated maintenance/backup rollout; [operations](cloud-push-operations.md) describes it. No live provider signing secret, producer token, or user payload is included in this record.

## Live qualification update — October 12, 22:25 UTC

- Exported a 675,688-byte encrypted pre-upgrade backup with the previous binary while maintenance had all stores/workers stopped. Downloaded it off-volume and restored into a fresh local quarantine. SQLite integrity checks passed for control and both workspace databases; the preserved pre-migration snapshots have control schema 100/workspace schema 6 and the restored copies have 102/7.
- Railway deployment `1a3ca770-5a80-45e6-b54b-3a675e282b2c` succeeded on merged commit `5f2d512`; public health returned 200. Existing polling resumed and Console Doctor showed schema 7 with healthy storage.
- Push admission is restricted to the two existing workspace IDs. Native adapters remain disabled. Public unauthenticated ingestion returns 401 `producer_required`.
- Created `push-qualification-20261012` through Console review/apply with `repo` grouping, `failed == true`, and no destinations. Authenticated submission is pending the limited producer credential; no successful live acceptance is claimed yet.
- Correction to the earlier deployment statement: a successful deployment from `main` did not prove automatic deployment. After merge, Railway still served `34ad00a`; its UI showed no connected branch and could not load branches. Reconnecting through the current CLI returned `main`, but the UI remains unresolved. A subsequent push-triggered deployment must verify this separately.
- One PR CI macOS Intel job rejected a concurrent local push with 400. The same test passed 20 local race runs; five further runs passed after adding response diagnostics. The cause is not confirmed and runtime behavior was not changed. Other duplicate CI jobs passed; this isolated failure is retained as evidence rather than called fixed.

Private backup files are under the operator's `~/.codex/ding-backups/push-20261012-2223/`; credentials and archives are excluded from Git. These checks do not complete the external destination, independent-user, restart, or 72-hour gates.

## Automated evidence

Local host: macOS arm64, Go 1.26.1. Linux disk and Railway lifecycle tests use disposable Docker containers/volumes. The test count below describes executed checks, not a throughput guarantee.

| Area | Evidence |
| --- | --- |
| Repository regression | `go test -race -tags console,mcpui -count=1 ./...` passed with Console and MCP assets built; subsequent changed-package regressions also passed |
| Type/build checks | Console TypeScript/Vite and MCP bundles built; `go vet ./...` passed |
| Unit browser packages | Console 13 tests and MCP app 7 tests passed |
| MCP interoperability | Independent TypeScript SDK exercised native stdio, 18 tools, review/apply, retry receipts, lifecycle/evidence, revocation, and independent daemon; 2 MCP app browser tests passed |
| JSON fuzzing | `FuzzStrictJSON`, 15 seconds: 79,618 executions, no failure; this is bounded smoke fuzzing, not sustained fuzz qualification |
| Tenant isolation | Same watch IDs and keys in separate workspace stores; foreign workspace headers; grant updates; safe diagnostic isolation; revoked producers denied |
| Resource grouping | 18 separate watch definitions accepted; one grouped watch with 18 repository incidents; recovering one leaves 17 open |
| Idempotency/lifecycle | Concurrent submissions commit once; different digest conflicts; rotation preserves identity; paused/edited watch exact replay; deletion/recreation fences old grants; expiry creates a distinct acceptance; durable usage survives restart |
| Reconciliation fixture | 1,000 unique inputs plus 100 duplicate retries and four store reopens reconcile without duplicate acceptance |
| Actual process loss | Subprocess killed before commit and after commit/before response; recovery produces exactly one receipt/observation and preserves the committed acceptance ID |
| HTTPS fairness | Real HTTP/2 TLS transport with a stalled chunked body: noisy workspace throttled, second workspace accepts, read deadline releases the body, health remains available |
| Disk faults | `scripts/filesystem-faults.sh`: actual ENOSPC in bounded 8 MiB tmpfs, actual read-only mount, no false acknowledgment, committed receipt replay on full disk, exact pending input accepted after space returns |
| Migration/recovery | Schema-100 credential migration preserves valid old service identity; failed metadata migration rolls back; quarantine revokes keys; workspace migration and existing single-writer/restart suites pass |
| Railway image | `Dockerfile.railway` built; `deploy/railway/qualify.py` passed mounted volume, non-root runtime, private files, owner/session persistence, exclusive writer, crash restart, wrong-key refusal, maintenance, encrypted backup, raw-snapshot quarantine, restored-session revocation |
| Provider adapters | GitHub signed workflow receipt/rotation/disconnect tests and Stripe SDK valid/tampered/stale-signature plus snapshot/account/mode tests passed; real provider delivery is still pending |

The final Railway qualification image was `sha256:16b8d23b7a7e535b2a338a97e74b4e8f0f66115b0ddba73f62bcf4148c3c238e` (local image ID, not a published registry digest). Its lifecycle/backup/restore qualification passed.

### Local HTTPS workload

`TestHundredWorkspacePushOverHTTPS` used 100 actual workspace SQLite stores, independent producer credentials, the real handler, TLS, and the fsynced producer ledger. Two seconds at 100 offered requests/sec produced:

| Measurement | Result |
| --- | ---: |
| Offered / accepted / durable usage total | 200 / 200 / 200 |
| Rejected / unconfirmed / generator busy | 0 / 0 / 0 |
| HTTP attempts | 200 |
| p95 / p99 acceptance latency | 26 ms / 31 ms |
| Driver elapsed | 2.023 s |

Every workspace received input. A later run during concurrent image builds exposed a timing-dependent minimum-offer assertion: all 90 offered requests were accepted, but the generator did not cover 100 workspaces in two seconds. The regression now verifies every workspace over HTTPS before the timed workload and reconciles both totals, without enforcing laptop throughput. This was a short local run with race instrumentation, synthetic bodies, and no external destination. It does not establish a 100 requests/sec Railway service level, memory headroom, long-term disk growth, or notification reliability.

### Browser qualification

The real cloud Console journey covers owner login, viewer denial, producer issuance with one-time secret dismissal, accepted JSON, rotation with receipt reconciliation, reload without secret recovery, pure JSON projection preview, distinct acceptance diagnostics, and producer revocation. The final journey also injects a lost rotation response and reconciles through metadata/revocation.

Chromium and WebKit run natively. The installed macOS Firefox fails before application startup with “Could not find profile folder”; Firefox qualification uses the repository's isolated Linux Playwright image. The full suites passed 25 tests each in Chromium, WebKit, and Linux Firefox, with two opt-in tests skipped per browser. The expanded final journey passed four additional Chromium/WebKit checks and two Linux Firefox checks. Strict MkDocs builds, including archived documentation versions, also passed.

## Release gates still requiring a deployed candidate

1. Take and verify an encrypted pre-upgrade backup, deploy this candidate from `main` under canary workspace admission, and record its commit/image digest and effective settings.
2. Run from a second host through Railway HTTPS, covering multiple independent AuthKit users/workspaces and shared-member roles. Reconcile input receipts and a durable external destination ledger separately.
3. Run the planned 72-hour workload, noise bursts, key expiry/rotation, restart, receiver outage, and snapshot restore. Record CPU/RSS, disk/WAL/backup growth, scheduler/delivery lag, recovery timing, and acknowledged data after the recovery point.
4. Verify external operational alerts and incident ownership. Publish capacity from that measured deployment, not the local result above.
5. Before enabling native routes, complete real GitHub workflow delivery/redelivery and Stripe test-mode snapshot delivery, secret rotation, account binding, and provider timeout behavior.

None of the elapsed soak, external provider, or Railway push acceptance results above are simulated as completed. See the [load driver and runbook](cloud-push-operations.md#qualification-driver) for reproducible next steps. Generic bearer push can be qualified independently of optional native adapters.
