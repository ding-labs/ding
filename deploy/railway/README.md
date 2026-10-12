# Ding Cloud on Railway

One always-running service, one region, one SQLite volume. Built-in named
accounts use **cloud** execution policy: public HTTP only, guarded remote
delivery, three watches, five-minute minimum cadence, and bounded retention.
Public signup and public MCP are disabled by default. Optional OAuth/MCP setup
and the deliberate existing-account transition are documented in
[Cloud MCP connections](../../docs/integrations/cloud-mcp.md). Supabase is deferred.

## Files and variables

Build the repository root with `Dockerfile.railway`. The container starts as
root only to prepare `/data` and `/run/ding`, then execs as UID/GID 10001 with
bootstrap and data-key variables removed from the serving process environment.
It refuses to start without an actual mount at `/data`. No pre-deploy command
can initialize this volume. Runtime config and key files are private and stay
outside the volume. The control database pins the login/hosting profile and a
fingerprint of the external encryption key; replacing that key fails startup.

| Variable | Value |
| --- | --- |
| `PORT` | `8080` (Railway target port; must be 1024–65535 except 8788) |
| `DING_PUBLIC_URL` | `https://cloud.ding.ing` |
| `DING_NAME` | `Ding Cloud` |
| `DING_DATA_KEY_BASE64` | Private base64 encoding of 32 cryptographically random bytes; stable across restarts/restores |
| `DING_MODE` | `serve` (default), `init`, or `maintenance` |
| `DING_STATE_NAME` | `ding` (default); selects `/data/ding`; a fresh simple name can select restored state |
| `DING_BOOTSTRAP_USERNAME` | Only with `init` |
| `DING_BOOTSTRAP_NAME` | Optional owner display name, only with `init` |
| `DING_BOOTSTRAP_PASSWORD` | Private 12–1024 byte input, only with `init` |
| `RAILWAY_VOLUME_MOUNT_PATH` | Injected by Railway; must be `/data` |

`init` creates the owner/workspace transactionally, removes its private password
file and starts serving. Once healthy, set `DING_MODE=serve` and remove **all**
bootstrap variables before redeploying. Repeated `init` fails; it never changes
an existing password. Do not use initialization for password recovery.

Keep the data key and the age backup identity in protected off-platform storage,
separately from archives. Losing the data key loses access to encrypted stored
credentials; a new key cannot repair that. Never put secrets in Git, command
arguments, an image, deployment logs, or the SQLite volume.

## Infrastructure and deployment

[`.railway/railway.ts`](../../.railway/railway.ts) uses Railway's current IaC SDK.
`railway.json`/`railway.toml` are deprecated and new services cannot opt into
them. The config pins an exact Git commit, one replica in `us-west2`, a 1 GiB
volume, 1 vCPU/1 GiB RAM limits, sleeping disabled, no pre-deploy command,
zero overlap and 60-second termination grace. These resource limits are not a
monthly billing cap. Leave project PR environments disabled.

Use a current authenticated Railway CLI. Link only the dedicated Ding Cloud
project/environment, then preview every IaC change before applying:

```sh
npm ci --prefix .railway --ignore-scripts
npm run check --prefix .railway
export DING_DEPLOY_COMMIT="$(git rev-parse HEAD)"
railway config plan
# Inspect the proposed resources, preserved secrets and pinned revision.
railway config apply
```

Secret values, operational mode and restored state selection are preserved by
IaC. Configure them privately before first deployment. A volume prevents
overlapping writers and introduces deployment downtime; do not add replicas.
Use a new qualified commit deliberately, and take a backup before changes that
alter persisted state. Code rollback does not roll back SQLite safely.

Register `cloud.ding.ing` with Railway and copy its exact CNAME and ownership
TXT records into Cloudflare. Start DNS-only. Railway terminates HTTPS and the
app accepts that one canonical Host/Origin. Optional Cloudflare proxying later
requires Full (strict) TLS and no caching of authenticated responses.

## Health and maintenance

Railway deploy healthcheck: **`GET /readyz`**. Public continuous monitor:
**`GET https://cloud.ding.ing/healthz`**. Both are 200 when the worker pool is
ready. In maintenance, `/readyz` is 200 to keep the maintenance container
available, while `/healthz`, UI and API return 503. Do not monitor `/readyz` as
proof that watches are running. Railway's deployment probe is not a continuous
monitor. The special `healthcheck.railway.app` Host is accepted only for these
two GET endpoints and cannot access accounts or the Console.

Set `DING_MODE=maintenance` and deploy. Confirm `/healthz` is 503 and deployment
logs say no stores or workers were opened. Normal execution must have stopped
before any offline operation. Maintenance opens no database and keeps Railway
SSH available on the same service/volume. Backup, restore, owner recovery and
deletion each acquire their own exclusive state lock and reject a live owner.

With the CLI linked to this service, run operators as the application user:

Use the **active** data directory from `DING_STATE_NAME` or the private runtime
config. The examples use the default `/data/ding`; after the October 11 hosted
restore drill this instance uses `/data/restored-20261011` instead.

```sh
railway ssh -- /sbin/su-exec 10001:10001 /ding-cloud backup \
  --data-dir /data/ding --output /data/backups/manual-YYYYMMDD.age \
  --recipient AGE_PUBLIC_RECIPIENT
railway volume files --volume ding-data download \
  /backups/manual-YYYYMMDD.age ./manual-YYYYMMDD.age
```

Choose a new archive name; exports never overwrite. Volume-file paths are
relative to the volume, so `/backups/...` is `/data/backups/...` in the container.
The archive is encrypted, but keep downloads private as well. Verify it using
a fresh-directory restore before treating an export as recoverable. Resume
`DING_MODE=serve` and deploy when done. Record the monitoring gap and downtime.

Railway SSH uses a registered SSH key; use a dedicated operator key if the CLI
asks. Do not expose the loopback metrics listener (127.0.0.1:8788) as a public
domain/port. Query it over Railway SSH or an authenticated local port forward.

## Portable restore and owner recovery

1. Select maintenance and stop **all** other copies that might own watches.
   Keep immutable deployment pinning in place; disable automatic redeploys.
2. Upload the encrypted archive to `/data/backups/`. Transfer the private age
   identity over authenticated SSH/SFTP to `/run/ding/restore.identity`, owned
   by 10001 with mode 0600. Keep it outside the volume and remove after use.
3. Restore into a **new** directory; do not pre-create that directory:

   ```sh
   railway ssh -- /sbin/su-exec 10001:10001 /ding-cloud restore \
     --data-dir /data/restored-YYYYMMDD --input /data/backups/manual-YYYYMMDD.age \
     --identity-file /run/ding/restore.identity
   ```

4. Restore creates `restore-required.json`, revokes old sessions/grants, pauses
   watches, cancels stale deliveries and retains evidence. Reapply deletions
   since the backup, inspect transfers and confirm no other runners are active.
   A normal server refuses quarantined state.
5. Release the reviewed copy explicitly:

   ```sh
   railway ssh -- /sbin/su-exec 10001:10001 /ding-cloud release-restore \
     --data-dir /data/restored-YYYYMMDD --confirm-other-runners-stopped
   ```

6. Set `DING_STATE_NAME=restored-YYYYMMDD`, retain the **original** data key, set
   `DING_MODE=serve` and deploy. Sign in again; review and resume watches
   individually. Release itself does not resume them or replay deliveries.

For a forgotten owner password, enter maintenance, write a new private password
file to `/run/ding/recovery.password` over authenticated SSH, and run:

```sh
railway ssh -- /sbin/su-exec 10001:10001 /ding-cloud recover \
  --config /run/ding/config.json --username owner \
  --password-file /run/ding/recovery.password
```

Remove that file, restore `serve`, deploy and sign in. Recovery preserves the
workspace and revokes the user's old credentials. Never pass the password as a
command argument or repurpose `DING_MODE=init`.

## Railway snapshots

Railway rejected snapshot creation and scheduling for the current workspace:
these features require **Pro**. No plan upgrade was made, and no automatic
snapshot schedule is active. Until that is enabled, recovery uses explicit
encrypted portable exports; changes since the latest export are not backed up.

When the workspace supports them, enable **daily volume backups** and take an on-demand snapshot before
state-changing releases. A raw snapshot can contain old active sessions and
pending notifications. Railway snapshot rollback is **not** Ding's restore
procedure. Before selecting a snapshot, deploy maintenance and verify the next
deployment will still use maintenance with no bootstrap variables.

Restore the Railway snapshot while staying in maintenance. Export its selected
state directory with `ding-cloud backup` into a new archive, then use the
portable restore steps above into a fresh directory on that restored volume.
Only switch the state name and return to `serve` after quarantine review and
release. This routes snapshot data through the same revocation and paused-watch
logic. Never simply roll the volume back under a serving runner.

Daily snapshots imply a nominal data-loss window of up to a day (longer after
failed/missed backups); portable exports have their own timestamps. Measure
actual recovery time and retain a record of what was lost or canceled.

## Verification and pilot gates

```sh
docker build -f Dockerfile.railway -t ding-cloud:railway .
python3 deploy/railway/qualify.py
```

This disposable-volume test checks mount refusal, non-root execution, private
runtime files, cookie security, sessions across redeploy/crash, second-writer
and changed-key refusal, maintenance, encrypted export, simulated raw snapshot
conversion, quarantine and restored-session revocation. It runs in CI alongside
the existing generic cloud image. Go backup tests also recover encrypted source
credentials and prove restored watches remain paused with stale deliveries
canceled. Browser tests cover cloud passwords, invitations and revocation.

On October 11, the actual Railway maintenance/export/download/restore drill
passed: a 417,064-byte encrypted archive was downloaded, restored into a fresh
volume directory and independently decrypted locally. Old browser sessions
failed, a saved encrypted credential matched, restored watches stayed paused
and no delivery was pending. The service resumed on `/data/restored-20261011`.
The synthetic watch and credential were removed after the drill. The active
owner watch checks `https://freedesign.md` every five minutes and records Console
incidents when `http.status >= 400`; no external destination is configured.

Before inviting a wider cohort, qualify actual Railway snapshot recovery,
real notification delivery with the laptop off, selected-watch transfer in both
directions and a 72-hour representative workload. Record health/check freshness,
oldest pending delivery, disk use, CPU/RSS, costs and recovery downtime. Local
container tests do not substitute for these hosted observations.

References: [Railway IaC](https://docs.railway.com/infrastructure-as-code),
[volumes](https://docs.railway.com/volumes),
[backups](https://docs.railway.com/volumes/backups),
[SSH and file transfer](https://docs.railway.com/cli/ssh),
[volume files](https://docs.railway.com/cli/volume).
