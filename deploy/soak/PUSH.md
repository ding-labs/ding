# Independent Cloud push soak

This runner tests generic bearer ingestion from Cloudflare to Railway for 72
hours. It uses a dedicated Worker, `ding-cloud-push-soak`, and the private
`push-soak/` prefix in the existing `ding-cloud-soak-evidence` R2 bucket.
It does not read or change the older HTTP soak's active pointer or schedule.

## Workload and evidence

- One immutable, durably recorded JSON batch per minute, containing 18 repository
  groups. The groups have staggered 15-minute failure/recovery phases.
- Exact replay every minute; changed-byte conflict every hour. Transport errors,
  429 and 503 retry the same bytes/key up to three times, with bounded deadlines.
- Each sample checks the receipt range, replay identity, all 18 group identities
  and states, watch revision continuity, and public health. Every five minutes it
  also captures workspace usage/Doctor and Railway CPU, memory, network and disk.
- Offers, claims, responses and configuration manifests are immutable. Duplicate
  Cron invocations cannot issue a second batch. A failed evidence write or lost
  invocation leaves an explicit gap; it is not reported as a success.
- The final scheduled batch is healthy. The producer stops at its hard deadline,
  after three failed samples, or at 85% workspace/volume storage usage. A stopped
  or incomplete run cannot pass merely because 72 hours elapsed.

Expected measured workload: 4,320 unique batches, 77,760 observations, 4,320 exact
replays, and 72 conflict checks. Warmup is recorded separately. This is a small
continuous correctness workload, not a throughput benchmark. It tests one actual
workspace; it does not qualify independent-user isolation, external notification
delivery, native provider adapters, backup-gap recovery, or all production gates.

## Prepare and deploy

Use the existing locked dependencies in `deploy/soak/worker`:

The push schedule is disabled after the stopped October 12 run. For a reviewed
new run, set `triggers.crons` to `["* * * * *"]` before deploying. At cleanup,
restore `[]` and apply with `wrangler triggers deploy --config wrangler.push.jsonc`
from the worker directory; keep the older HTTP runner's config unchanged.

```sh
npm ci --prefix deploy/soak/worker
npm run test:push --prefix deploy/soak/worker
# From deploy/soak/worker:
npx wrangler deploy --config wrangler.push.jsonc
```

Create `push-soak-20261012` through Console review/apply with `source.type: push`,
`itemsPath: events`, `fields: {repo: repo, failed: failed}`, `groupBy: [repo]`,
`failed eq true`, transition policy, and no destinations. Create five-day
credentials: one ingest-only grant for that watch and a separate inspect-only
grant for the workspace. Revoke both after the run.

Private operator state is `~/.config/ding-cloud/push-soak/` (0700). Keep
`worker-secrets.json` (0600) containing `ADMIN_TOKEN`, `PRODUCER_TOKEN`,
`INSPECT_TOKEN`, and the existing production-scoped `RAILWAY_TOKEN`. Upload using
`wrangler secret bulk PRIVATE_FILE --config wrangler.push.jsonc`; never put
secret values in arguments, Git, or reports. The existing Railway project token
is shared with the older observer and must not be revoked as this run's cleanup.

Before starting, verify the encrypted backup/restore evidence, actual Railway
application commit, health, loopback metrics, storage headroom, and scoped metric
query. Save these under the private operator directory. Wait for an actual Cron
invocation to appear; trigger propagation can take several minutes.

```sh
python3 deploy/soak/scripts/push.py status
python3 deploy/soak/scripts/push.py warmup --application-revision ACTUAL_LIVE_COMMIT
python3 deploy/soak/scripts/push.py export
python3 deploy/soak/scripts/push.py start
```

Warmup has a ten-minute hard deadline. Measurement requires at least three
complete accepted samples, no missing/failed elapsed sample, and a successful
five-minute telemetry sample. A new T0 is the next minute after the start request,
not deployment time. Inspect remote status after a lost start response; never
start twice or replace a failed run. A failed warmup needs diagnosis and a
reviewed fresh-run transition, not a forged pass.

## Follow-up and finish

The Worker runs without this laptop. The Codex heartbeat requires the host to be
available. Use `push.py status` and `push.py export` hourly; the export is
incremental and retains failures. Inspect receipt/state/telemetry evidence, not
just the summary. Check current Railway deployment against the recorded commit;
record and stop on an unplanned application change. Avoid merging application
changes during measurement. Record SSH process/loopback/storage checkpoints at
T0, 24h, 48h and completion. Leave existing watches and the older soak unchanged.

Do not inject faults or restart production casually. Record any separately
planned restart/outage and its actual impact; missing samples remain visible.
The no-destination watch cannot verify notification delivery.

At the end, export all evidence, reconcile accepted batch/observation totals
against durable workspace usage, verify final entity recovery, pause only the
soak watch, revoke its two Ding grants, disable only the push Worker's Cron,
and remove its producer/inspect secrets. Preserve private evidence and manifests
for the bucket's existing 30-day retention; export before expiry. Publish a
sanitized report with exact T0/end, deployed revisions, missing/failed samples,
latencies, resource trends, and all untested gates. Pause the push heartbeat only
after reporting and cleanup. Never claim a successful run from elapsed time alone.
