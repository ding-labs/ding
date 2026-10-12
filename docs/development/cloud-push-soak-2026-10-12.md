# Cloud push soak — 2026-10-12

Run: `push-8886f49596c30ebc`. This is an in-progress measurement, not a
completed qualification result.

## Recorded configuration

- Measurement starts October 12, 2026 at 23:19 UTC (4:19 PM Pacific).
- Hard deadline: October 15, 2026 at 23:19 UTC (4:19 PM Pacific).
- Railway application: `f8cd4f8b8d55f792f2e3674c68100acec1eea137` on `main`.
- Railway deployment: `79255a90-0855-494c-a6ba-55cb87dc1a02`.
- Observer source: `2cb7a6bb429c057f7dc9c6f33d8e27af5478c346`.
- Worker version after secret installation: `cd6137ef-2afc-402c-aa32-463adfbcbfb0`.
- Watch: `push-soak-20261012`, revision prefix `b9200daeefe5`.

## Startup evidence

The actual scheduled warmup began at 23:15 UTC. Its first three batches passed:
18 observations and matching entity states each, exact replay with the original
acceptance identity, and healthy public responses. Acceptance latencies were
297, 189, and 191 ms. The five-minute telemetry sample also passed. The server
accepted the measurement transition only after three complete elapsed intervals
with no missing or failed samples.

The first measured batch at 23:19 UTC passed with all 18 groups, a matching
202 replay, and HTTP 200 health. Acceptance took 203 ms. Its immutable evidence
was exported locally; no stop condition was present.

Both five-day credentials were verified against the live API before upload:
the observer could inspect but could not ingest, and the producer could not
inspect. Credential values and raw evidence remain in private operator storage.
The pre-warmup SSH checkpoint showed zero failed workers, zero pending
deliveries, 36,624 KiB RSS, and 3% volume use.

## Scope and follow-up

See [the operator runbook](../../deploy/soak/PUSH.md) for workload, evidence,
stop conditions, reconciliation, and cleanup. The independent Worker runs one
18-group batch per minute, exact retries, and hourly changed-byte conflicts.
Hourly follow-up is configured, with runtime checkpoints at 24h, 48h, and finish.

This tests one workspace without notification destinations. Independent-user
isolation and external notification delivery remain separate qualification
gates. Preserve the application revision during measurement; a changed revision
must be recorded as an interrupted run. Publish the final verdict only after
exporting and reconciling the complete evidence.
