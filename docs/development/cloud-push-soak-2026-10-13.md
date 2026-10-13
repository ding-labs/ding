# Cloud push rerun — 2026-10-13

Run `push-04da8c7b69a1524a` preserves the failed October 12 attempt as
`previousRunId: push-8886f49596c30ebc`. Its evidence remains under a separate
immutable run prefix. This document records startup, not a completed pass.

## Verified setup

- Railway application: `f8cd4f8b8d55f792f2e3674c68100acec1eea137`, unchanged.
- Observer source: `c5cef766ba84853db88a6b63b27fa14e20bbe074`.
- Worker version: `c64947a9-23f9-4aee-8859-6c860770b233`.
- Watch: `push-soak-20261012`, **Push 72-hour soak (temporary)**, resumed in
  Personal workspace; revision prefix `b9200daeefe5`.
- Producer: `push-soak-producer-20261013-rerun`; single key `9de79fc44922…`,
  expires October 18 at 18:23:38 UTC (11:23:38 AM Pacific).
- Inspector: `push-soak-inspect-20261013`; single key `06a91d9abef6…`,
  expires October 18 at 18:20:52 UTC (11:20:52 AM Pacific).

Secrets were installed directly as encrypted Cloudflare Worker secrets. The
deployed credential probe confirmed producer 403 / inspector 200 on the read-only
status endpoint. Warmup began at October 13, 18:26 UTC. The producer key metadata
then recorded accepted use at 18:26:18 UTC, proving the installed key was the
five-day key above. There is no producer overlap key. Inspector authentication
succeeded against its sole key; its UI does not record accepted-use metadata.

The restart transition, evidence preservation, required fresh warmup, and deployed
credential checks passed all 12 push tests, including the Workers runtime tests.
The pre-warmup SSH checkpoint found zero failed workers, zero pending deliveries,
38,792 KiB RSS, and 3% volume usage. Hourly follow-up is active for this run.

## Measurement

All five recorded warmup batches passed, including telemetry, with no missing
or failed elapsed samples. The measured window is October 13 at 18:31 UTC through
October 16 at 18:31 UTC (11:31 AM Pacific on both dates). Both keys remain valid
for nearly two days beyond the deadline.

The first measured batch passed: 18 matching groups, identical accepted replay,
HTTP 200 health, and 283 ms acceptance latency. Its evidence was exported.
At measurement start, workspace ingest usage was 74 unique requests and 1,009
observations, including the earlier attempts and this run's 90 warmup observations.
Hold the application revision stable until completion or explicitly record an
interrupted run; the 72-hour outcome remains pending.

See [the runbook](../../deploy/soak/PUSH.md) for workload, stop conditions,
checkpoint schedule, reconciliation and cleanup. This remains a single-workspace
test with no external notification destination; other release gates remain open.
