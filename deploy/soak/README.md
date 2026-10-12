# Ding Cloud owner soak

This harness implements the [72-hour plan](../../docs/development/railway-cloud-soak-plan.md).
The independent Cloudflare Worker samples public health every minute, reads
watch state/events outside hours 12–18, and collects Railway metrics and Ding
usage/Doctor every five minutes. R2 holds private immutable evidence and a
durable event cursor. Fixed fixtures exercise HTTP statuses, header timeouts,
invalid JSON and projection-size limits. No external alert destination is used.

## Current resources

- Worker: `ding-cloud-soak`, account `b5a19010c0fa72255f749f63b2ec0962`.
- Worker hostname: `https://ding-cloud-soak.zuchka.workers.dev`.
- Private bucket: `ding-cloud-soak-evidence`; public access disabled, objects
  expire after 30 days. Export before expiry; local copies need operator cleanup.
- Railway: Ding Cloud / production / ding-cloud, pinned application revision
  `c5d9213eef4ad8049ef90d4d631e14eb6b419fc1`, one replica, sleeping disabled.
- Private operator directory: `~/.config/ding-cloud/soak` (0700); JSON, tokens
  and archives are 0600. `active.json` is the authoritative local run pointer.
- Codex heartbeat: `ding-cloud-soak-follow-up`, hourly in the implementing chat.
  It checks completion and reports findings, staying quiet during normal operation.

Read live `/admin/status` and `active.json` for actual times. **Deployment and
warmup are not a started 72-hour measurement.** The cloud sampler does not need
the laptop. Operator follow-ups require the Codex host to be available.

## Build and verify

```sh
npm ci --prefix deploy/soak/worker
npm test --prefix deploy/soak/worker
python3 -m py_compile deploy/soak/scripts/*.py
go test ./deploy/soak/verify
```

The tests use the actual Workers runtime and R2 for scheduled outbound requests,
authentication, fixture and
conditional-write behavior, plus fault tests for interrupted evidence storage,
duplicate Cron invocations, bounded decoding, missing samples and false reports.
The lockfile pins Wrangler and its current Miniflare runtime. `npm run types`
generates local binding/runtime declarations; generated declarations are ignored.
The source entry exports only the Worker handler; helper exports are test-only.

## Credentials and deployment

`prepare.py` creates a seven-day Ding inspect-only service token and a random
Worker admin token. It checks a read succeeds and a mutation receives 403. It
refuses to overwrite an existing run. Inspection is workspace-wide. Owner
credentials are used locally for provisioning, never installed on the Worker.

Create a Railway project token scoped to production and save it to
`~/.config/ding-cloud/soak/railway-project-token.txt`. Run `metrics.py` to test the
fixed GraphQL query and save the token privately. No personal-token fallback is
allowed. Project tokens are not inherently read-only; this Worker has only a
fixed metrics query. `User-Agent` is explicit because provider edges can reject
Python's default agent before the request reaches the API.

Deploy `worker/wrangler.jsonc`, with `ADMIN_TOKEN`, `DING_TOKEN` and
`RAILWAY_TOKEN` as secrets, and the R2 binding. The connected Cloudflare API can
upload the dry-run module with equivalent metadata if local Wrangler is not
logged in. Never put secret values in command arguments, Git or logs. Enable
the one-minute Cron; propagation can take 15 minutes. Verify authenticated status
works, unauthenticated status is 404, and actual scheduled samples arrive.

## Preflight and dry run

1. Run `checkpoint.py`; verify live revision/replicas/sleep settings with Railway.
   It resolves the actual state directory and saves loopback metrics, process
   RSS and storage sizes without exposing the private metrics listener.
2. Record the quota budget in `preflight.json`: three watches, about 2,800 extra
   checks including warmup and roughly 20 MiB acquired bytes. Verify monthly
   usage and remaining provider quotas. Record provider billing at T0/daily.
3. Create a synthetic vault secret named `SOAK_RECOVERY_PROBE` through the owner
   API and save `{name,value}` privately in `recovery-probe.json`.
4. Stage only `DING_MODE=maintenance`, review staged changes, accept the deploy,
   and verify a new deployment actually starts. If Railway commits the variable
   without a deployment, redeploy the known pinned build explicitly. Wait for
   `/healthz` 503 and `/readyz` 200; confirm maintenance logs before proceeding.
5. Run `backup.py`. It exports offline, downloads an encrypted archive, restores
   to a new local directory, verifies quarantine, paused watch, no pending
   delivery, and synthetic-secret decryption with the original data key. Never
   release or serve that restored copy. Resume `DING_MODE=serve` with the same
   reviewed deployment procedure and verify health/checks recover. Remove the
   live synthetic secret after verification. Keep the private probe for later
   verification of this archive.
6. `preflight.json` must have real evidence with `verdict: pass` for
   `backupRestore`, `metrics`, `workerDeployed`, `quota`, `runtimeCheckpoint`.
   Do not manufacture these attestations from elapsed time or a command launch.
7. Commit the tested observer; deploy that exact bundle. Run
   `python3 deploy/soak/scripts/operate.py warmup`, then `apply-watches`. The
   apply command records previews and uses their review objects. It preserves
   the baseline revision of `freedesign-http`.
8. Warmup has a three-hour hard limit. Review after at least one complete
   90-minute cycle; it must include two actual requests in every phase for both
   watches. Run `operate.py export` to download incremental evidence and compute
   the gate. Start only while synthetic watches have recovered and sampling is
   reliable. A failed gate stays visible; diagnose it rather than bypassing it.
9. Run `operate.py start` after the gate passes. This records a fresh T0 and
   T0+72h, independent of deployment/warmup time. Reconcile a successful remote
   start if interrupted before the local pointer was saved; do not start twice.

An initial Cron propagation gap can invalidate warmup coverage. Preserve that
warmup and use `operate.py restart-warmup` after qualifying and deploying the
fix. It exports the attempt, deletes only the test watches and creates a fresh
run ID/epoch and unique watch IDs (deleted Ding IDs cannot be reused). The scoped operator credentials remain within their original
expiry until final cleanup; never backdate T0 or hide missing samples. Application/observer fixes
must be qualified before a new measurement. Immutable keys and conditional
writes prevent retries from fabricating extra samples. A failed write leaves a
visible gap, and event cursors advance only after corresponding evidence is saved.

## Follow-up procedure

Use `operate.py status` and `operate.py export`. Export writes a private report
under `~/.config/ding-cloud/soak/evidence/RUN_ID/MODE/report.json`. Read all gate
details, not just the top-level verdict. The report cannot pass on elapsed time
alone. Any safety stop, telemetry errors, missing evidence, duplicate/missing
incidents or unresolved resource excursions must remain in the final findings.

During hours 12–18 do not open the Console, log in, run `checkpoint.py`, or make
other authenticated Ding requests. Worker health and Railway sampling and
fixture logs continue. The Worker reconciles events after the quiet period.
The operator's evidence reads are against Cloudflare, not Ding.

At 24/48/72 hours, save private runtime metrics and provider usage/billing.
Schedule the pinned redeploy and maintenance/export in working hours 24–48.
Before each outage, POST `{start: UNIX_MS, end: UNIX_MS}` to authenticated
`/admin/maintenance`: start must be at least 30 seconds in the future, and all
windows total at most ten minutes. The server records declaration time; reports
do not excuse retroactive windows. Then carry out the reviewed Railway actions.
Record actual down/up times, pinned revision/image, preserved watch revisions,
first subsequent checks, archive checksum and download verification. Keep
`freedesign-http` running. Record these observations under `drills.persistence`
in private `drills.json`; use pass/fail/inconclusive and concrete evidence paths.

During hours 48–72, use a disposable viewer membership/browser/device and a
separate test service credential to prove revocation. Never revoke the observer
or owner. Follow existing cloud auth APIs/tests, save only result metadata, and
remove the temporary access. Verify the exported archive again on an isolated
copy; use `deploy/railway/qualify.py` and `scripts/filesystem-faults.sh` for
crash/storage tests on disposable volumes. Record `drills.accessRecovery` with
evidence, including restored-session revocation and durable history during the
quiet window. Store `drills.resourceReview` and `drills.billing` separately.
No completed drill attestation means that part remains inconclusive.

Three failed health probes, overdue watches by five minutes, or usage above 90%
set a durable safety stop. Fixtures immediately become healthy; the observer
continues collecting. Do not clear the stop. Investigate, preserve the failed run
and stop further operator fault injections. A correctness issue in the report
also stops further drills even if the cloud safety stop has not fired.

## Finish and report

At the exact end, fixtures return healthy automatically. Sampling continues for
ten minutes to capture recovery, then stops even if the laptop is unavailable.
After at least six minutes, inspect recovery and run `operate.py cleanup`. This
checks healthy synthetic entities, deletes only the two test watches, exports
evidence and revokes the Ding observer token. It never deletes the real watch.
Then disable only this Worker's Cron through Cloudflare, revoke the Railway
project token in the project, remove test users/credentials, and export once
more if required. Keep private evidence for 30 days; delete local private run
artifacts after that retention period. Do not delete unrelated owner recovery
keys or archives.

Produce a sanitized report in `docs/development/` with exact measurement times,
application/observer revisions, all gate verdicts, missing evidence and downtime,
check/incident counts, memory/disk plots, actual provider usage/cost, checkpoint
results and remaining release gates. Raw probe failures measure sampled minutes;
use operator/deployment timestamps to report outage bounds and exact known
maintenance durations. Distinguish container RAM from process RSS. The scheduling
summary covers observable due times, not every check in the quiet window; verify
that history in the isolated backup before qualifying unattended execution.

Send findings in the original chat and pause `ding-cloud-soak-follow-up` after
cleanup. Do not claim that 72 hours qualifies external delivery, snapshot
recovery, transfer, seven-day retention, or wider-user capacity.
