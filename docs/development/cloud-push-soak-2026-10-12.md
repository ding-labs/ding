# Cloud push soak — 2026-10-12

Run: `push-8886f49596c30ebc`. **Failed / stopped early.** The 72-hour
qualification was not completed.

## Failure and reconciliation

The runner stopped at October 13, 00:07:16.807 UTC (October 12, 5:07 PM Pacific)
after three consecutive `401 service_revoked` responses at 00:05–00:07 UTC.
The operator had installed the previous producer key from the clipboard. It was
valid during warmup but expired at 00:04:58 UTC after its one-hour rotation overlap.
Console key metadata showed its last accepted use at 00:04:15 UTC; the replacement
five-day key had no recorded accepted use. Permission-only preflight missed this.

- 49 consecutive measured samples: 46 passed, then three authorization failures;
  no missing sample before the automatic stop.
- 46 unique accepted batches, 828 observations, 46 matching exact replays,
  and one changed-byte conflict correctly rejected with 409.
- All accepted samples matched the expected 18 group states. Ten telemetry
  samples passed; every recorded health request returned 200.
- Accepted-request latency: 169 ms minimum, 202 ms median, 307 ms maximum.
- Durable usage rose from 19 to 919 observations: exactly 828 measured plus
  72 from four warmup batches. Unique request usage rose from 19 to 69.
- During sampled telemetry, live data grew from 475,136 to 1,101,824 bytes;
  Railway memory from 0.0243 to 0.0311 GB and disk from 0.0643 to 0.0682 GB.
- Railway stayed on the recorded application revision. The later SSH checkpoint
  found zero failed workers, zero pending deliveries, and 3% volume use.

The remaining 4,271 scheduled batches were not executed. Final recovery was not
reached: 15 synthetic entities remained open when the watch was paused. Preserved
state and immutable evidence must not be represented as a successful 72-hour run.
Both Ding grants were revoked, the push Cron disabled, and its producer/inspect
Worker secrets removed. The older HTTP runner and shared Railway token were kept.

The runbook now requires verification of the **actually used key's** expiry before
measurement. A new run needs new credentials, a reviewed fresh-run transition
that preserves this attempt, warmup, and a new 72-hour clock.

## Recorded configuration

- Measurement starts October 12, 2026 at 23:19 UTC (4:19 PM Pacific).
- Originally planned deadline: October 15, 2026 at 23:19 UTC (4:19 PM Pacific).
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
Hourly follow-up was configured, with checkpoints planned at 24h, 48h, and finish.

This tests one workspace without notification destinations. Independent-user
isolation and external notification delivery remain separate qualification
gates. The exported stopped-run evidence supports the failure report above; it
does not establish long-duration stability or those remaining release gates.
