# Railway owner pilot: 72-hour soak

Status: implementation in progress; measurement has not started. Prepared
October 11, 2026, after PR #34 merged. The harness and follow-up procedure are
in [deploy/soak](../../deploy/soak/README.md); actual start/end times live in the
private run manifest. See the [cloud plan](railway-cloud-plan.md) and
[operations runbook](../../deploy/railway/README.md).

## Objective and scope

Qualify three days of unattended execution for the current owner instance at
`https://cloud.ding.ing`. Keep Railway, one replica, SQLite and the existing
cloud limits. Alerts remain in the Console. The result is a measured owner-pilot
report, with separate pass/fail/inconclusive decisions for each tested behavior.

The current deployment has already passed CI, browser account tests, restart
persistence and a hosted portable backup/restore drill. The soak adds evidence
about recurring execution, incident correctness and resource trends over time.

External notification delivery/retries, live watch transfer, actual Railway
snapshot recovery and multi-workspace capacity remain separate qualifications.
Seventy-two hours does not exercise the full seven-day retention window.

## Components to build

1. **Independent observer and test endpoints:** one small Cloudflare Worker
   with scheduled sampling and two fixed synthetic endpoints. A one-minute
   Cron Trigger samples Ding's public health; the Worker continues while the
   laptop is off. Cloudflare documents minute schedules and notes that trigger
   changes can take up to 15 minutes to propagate. Verify actual arrivals before
   starting the clock. [Cron Triggers](https://developers.cloudflare.com/workers/configuration/cron-triggers/)
2. **Private evidence:** an R2 bucket bound to that Worker stores run metadata,
   timestamped probe results, synthetic request records and checkpoint reports.
   Use a private bucket and authenticated report access. Store compact metadata,
   never passwords, tokens, cookies or response bodies. [R2 Worker bindings](https://developers.cloudflare.com/r2/api/workers/workers-api-usage/)
3. **Operator scripts:** prepare manifests and credentials, take resource
   checkpoints, execute the attended restart/backup/access drills, download
   evidence, produce the report and clean up the temporary watches.
4. **Tests for the harness:** exercise fixture phases, bounded collection,
   interrupted writes, duplicate scheduled invocations, missing evidence and
   incident comparison. A broken observer must not produce a passing report.

Proposed locations: `deploy/soak/worker/`, `deploy/soak/manifests/`,
`deploy/soak/scripts/`, and `deploy/soak/README.md`. Keep run artifacts outside
Git; commit only a sanitized final summary. Use the Worker's default hostname
for fixtures so no new custom DNS record is required.

The observer makes read requests only. Restart, backup, restoration and access
changes remain explicit operator actions. No new application metrics endpoint
is needed: the existing operator `/metrics` stays on loopback.

## Three-watch workload

Use the existing three-watch allowance and five-minute minimum cadence. Recheck
available slots and remaining monthly budgets immediately before preparation.
All three watches have no delivery destinations.

| Watch | Workload | Expected behavior |
| --- | --- | --- |
| `freedesign-http` | Existing real watch, unchanged | HTTP status >= 400 opens an incident; a healthy response recovers it. Real site errors are recorded separately from Ding failures. |
| `soak-status` | Synthetic status endpoint, five-second timeout, status >= 400 | Repeat 15-minute phases: 200, 400, 500, 200, seven-second delay before headers, 200. One firing across contiguous 400/500 phases, a recovery at 200, source-error/recovery events for the timeout. |
| `soak-projection` | Synthetic JSON endpoint, bounded `value` field projection, condition `value >= 1` | Repeat 15-minute phases: valid 0, valid 1, malformed JSON, valid 0, response above 64 KiB, valid 0. Bad input becomes unknown and holds the existing incident; valid input restores source health and evaluates the condition. |

Use transition policy, consecutive=1, recoverAfter=1, onUnknown=hold-incident,
and explicit cloud-compatible bounds. Give some status-only responses a body
around 140 KiB to cover the freedesign regression: status acquisition must work
without consuming that body. The projected-body watch must still reject input
above its limit. Keep slow-response phases below the platform's source timeout
limit but above the watch's configured timeout.

Fixtures use a recorded UTC epoch and deterministic phase schedule. Each actual
request records its run ID, time, selected phase and intended response. Set
`Cache-Control: no-store`; omit conditional-cache validators. Endpoints accept
only fixed test paths and GET, with bounded delays and payloads. No public
mutation endpoint or arbitrary outbound fetch is needed.

Evaluate expected transitions against actual request/observation sequences,
including requests crossing phase boundaries. Do not infer an expected event
solely from the observer's polling time. More than one check sees each phase.
Roughly 864 checks per watch is a planning estimate: scheduling follows each
completed check, adds bounded jitter, and includes declared maintenance gaps.

## Evidence and credentials

| Collection | Cadence and data |
| --- | --- |
| Public health | Every minute: `GET /healthz`, result, latency, timeout/TLS error and actual sample time. `/readyz` is not a serving-health signal during maintenance. |
| Watch execution | Every minute outside the quiet window: existing watch-detail/status APIs, including `lastInputAt`, `nextAt`, generation, source health and incident state. |
| Incident evidence | Incrementally page `/v1/events` and event observations using durable cursors; archive IDs and relevant synthetic fields. Handle pagination and bounded catch-up explicitly. |
| Usage and storage | Every five minutes: `/v1/cloud/usage` and existing Doctor data. Export Railway CPU, memory, disk and network series; record billing/usage at start and daily checkpoints. |
| Private runtime metrics | At attended checkpoints and drills: existing loopback metrics over Railway SSH, process RSS and active SQLite/WAL sizes. Record failed workers, scheduler lag and pending deliveries. |
| Observer health | Record scheduled time, actual start/finish, storage outcome and collector version. Report missing samples and telemetry errors separately from Ding errors. |

Create a separate Ding service credential with only `inspect` scope, seven-day
expiry, and workspace access. Inspection is workspace-wide in the current
implementation; the watch-ID allowlist is an ingest restriction, not an inspect
filter. Prove reads work and writes are denied before using the credential.
Keep it in Worker secrets and revoke it during cleanup. The observer never uses
the owner's password or browser session.

For Railway collection, first validate the needed metrics queries with a token
scoped to this project's production environment. Project-token authentication
uses `Project-Access-Token`. The collector issues only fixed read queries; an
environment-scoped token is not necessarily read-only. Do not silently fall back
to a broad personal token. If a metric is unavailable through the scoped API,
resolve collection through supported exports before T0 or mark that gate
unqualified. [Railway API](https://docs.railway.com/integrations/api)

Railway exposes infrastructure metrics, while application timing and correctness
come from Ding and the fixture records. Name memory measurements accurately:
container RAM and process RSS are different series.
[Railway metrics](https://docs.railway.com/observability/metrics)

Use unique run IDs and immutable sample keys, deduplicate retried invocations,
and persist event cursors only after evidence is stored. Bound requests, catch-up
pages and stored bytes. One run needs about 4,320 health samples, 864 resource
intervals and 2,600 source checks. Verify projected usage against the remaining
30,000-check/128 MiB monthly workspace budget, including oversized-body phases,
and current provider quotas before provisioning. Record actual costs; do not
promise a free run or upgrade a plan as part of this test.

## Preparation and start gate

- Confirm the deployed revision, active state directory, one-replica setting,
  disabled sleeping, HTTPS, available watch slots and current usage. The active
  directory from the original recovery drill is `/data/restored-20261011`;
  resolve it again rather than assuming it remains unchanged.
- Build and test the observer/fixtures/report generator. Verify Cloudflare
  Workers/R2 access and Railway metric access; DNS access alone is insufficient.
- Take and download a fresh encrypted backup using maintenance mode, resume
  service, and validate restoration into a separate quarantined directory.
  Seed a uniquely named synthetic vault credential for this drill; retain its
  expected value privately, verify it with the original data key in the restored
  copy, and remove the live synthetic credential afterward.
- Review/apply the two synthetic watches through the existing preview/apply
  protocol. Record the original real-watch revision and all test resource IDs.
- Exercise one complete fixture cycle before measurement and compare all
  expected incidents/source errors. Verify independent health collection and
  private storage. This dry run is not part of the 72 hours.
- Record T0, T0+72h, code/image/observer revisions, fixture epoch, thresholds,
  checkpoint times and planned maintenance windows. Publish the run manifest
  privately. Start only after the observer is reliably sampling.

Keep the application revision fixed. A same-revision restart is a planned test.
A code fix changes the subject under test: preserve the failed run, qualify the
fix and start a fresh 72-hour window. No backdated start time.

## Execution schedule

| Period | Work and evidence |
| --- | --- |
| Hours 0–24 | Normal three-watch execution and repeated fixture cycles. Include a six-hour quiet window, e.g. hours 12–18, with no Console use or authenticated Ding inspection. Continue public health and independent fixture request records; reconcile durable observation history afterward. Record the first daily resource/usage checkpoint. |
| Hours 24–48 | One attended restart/redeploy of the pinned revision. After stable checks resume, one attended maintenance/export/resume drill. Record start/end, health gap, watch revisions, first subsequent checks, archive checksum and off-platform download. Schedule these within operator working hours; declare the exact windows in advance. |
| Hours 48–72 | Continue steady execution. Create a temporary viewer, verify read-only access, revoke membership and test existing browser/device access. Separately revoke a test service credential; leave the observer credential active. Validate the exported archive on an isolated copy and run storage-pressure/crash cases on disposable storage. Finish with a final resource checkpoint and report. |

The observer runs in the cloud; the laptop is needed only for attended operator
actions. Synthetic endpoint failures never require changing freedesign.md.
Storage exhaustion, forced crash and restore drills use disposable copies.
Production's planned interruptions are the restart and portable export.

## Proposed pass criteria

These thresholds are test gates for this small workload, not a published SLA.
Freeze them in the run manifest before T0.

| Gate | Pass requirement |
| --- | --- |
| Evidence coverage | At least 99% of expected minute samples, with no unexplained gap longer than five minutes. The intentional authenticated-read quiet window is covered by fixture records and subsequent durable history. Missing evidence that prevents evaluation makes the result inconclusive. |
| Availability | No unexplained health failures outside predeclared maintenance. Planned interruptions total at most ten minutes; publish raw downtime as well as maintenance-adjusted results. Attribute transient observer/network errors rather than deleting them. |
| Scheduling | At least 99% of observable due checks complete within 60 seconds after recorded `nextAt`; no unexplained overdue check longer than one five-minute interval. Account for the source timeout and intentional downtime, and report every exception. |
| Correctness | Every observed synthetic transition has the expected firing/recovery or source-health event; no duplicate incident opening, false recovery on unknown data, missing evidence or invented HTTP status. |
| Persistence | Same revisions and committed history survive restart; checks resume within one interval plus 60 seconds after service returns. No lost watches, corrupted SQLite state or unexpected worker failure. |
| Console-only behavior | No destination configured; pending-delivery count remains zero. This does not qualify external delivery or retries. |
| Resources | No OOM, quota exhaustion or unexplained sustained growth. Investigate memory above 512 MiB for 15 minutes, CPU above 0.5 core for 15 minutes, or volume usage above 70%; unresolved excursions prevent a pass. Model normal retained-history growth separately from a leak. |
| Access and recovery | Revoked test users/tokens lose access. Isolated restore rejects old sessions, retains evidence, pauses watches and cancels stale deliveries. Original encryption key recovers a synthetic credential in the recovery fixture. |

Three consecutive failed health probes, a failed worker, a watch overdue by an
entire interval, a correctness mismatch or imminent quota exhaustion opens a
run issue. Stop additional fault injections, preserve evidence and investigate.
Do not repeatedly redeploy or hide a failure by clearing state. An observer
outage also opens an issue; it cannot certify its own missing interval.

## Reporting and completion

Generate private checkpoints at 24, 48 and 72 hours, showing completed checks,
health/observation gaps, incident comparisons, resource trends, costs and open
issues. Under the current Console-only preference, no Slack/email/webhook
notification is configured. Operator review of the independent report is needed
to learn about an outage when Ding's Console is unavailable. Optional Codex
follow-ups are now configured through the hourly `ding-cloud-soak-follow-up`
heartbeat, which reports meaningful failures and final findings.

At 72 hours, return synthetic endpoints to healthy responses, wait for a healthy
observation and expected recovery, then pause/delete only the synthetic watches
through guarded lifecycle operations. Keep `freedesign-http` running. Stop the
soak-specific schedule, revoke temporary credentials and remove the test user.
The Worker must stop test activity at its recorded end time even if cleanup is
delayed. Export evidence before removing infrastructure; retain private run
artifacts for 30 days and keep the sanitized report in the repository.

The final report includes exact revisions/times, each gate's verdict, all
maintenance and unexplained gaps, charts for check timing/memory/disk, recovery
evidence, actual usage/cost and remaining release gates. A failed or incomplete
gate stays visible. The outcome may qualify the Console-only owner workload;
external delivery, transfer, snapshot recovery, longer retention and wider-user
capacity are still listed separately.
