# Operate a limited cloud qualification instance

This is an operator runbook for the [source preview](cloud-preview.md), not a
published service or spending approval. One process owns one persistent host
volume. There is no active/active mode or lease-based failover. Start with a
restricted cohort only after the local pilot, identity/provider qualification,
representative soak and an approved operating budget.

## Deploy one worker

Build `ding-cloud` with `console,mcpui` after both asset builds. Provision a dedicated
non-root `ding-cloud` user, install the binary at `/opt/ding-cloud/ding-cloud`, and
create an owner-only `/etc/ding-cloud/config.json` using the preview configuration.
Add `operatorListen: "127.0.0.1:8788"` and `enrollmentLimit: 0` initially. The key
file must contain 32 cryptographically random bytes, be owned by that service user,
mode 0600, and live outside `/var/lib/ding-cloud` and the backup filesystem. Keep a
protected recovery copy off the host. Do not print the key in logs or shell history.

Review `deploy/cloud/ding-cloud.service`, install it through systemd, and validate
with `systemd-analyze verify`. It grants writes only to the state directory/private
temporary files and caps resources. Treat its 1 GiB memory limit as a starting
safety bound to measure, not a capacity claim. Verify credential ownership and
`ding-cloud --config /etc/ding-cloud/config.json --check-config` under the actual
service user before starting. Shape validation does not test live identity.

Place a validated HTTPS reverse proxy in front of the loopback listener. The
example `deploy/cloud/Caddyfile` preserves the exact public Host and excludes
access logging of OAuth callback queries. Configure the real DNS name/TLS, validate
the proxy configuration, and test the public certificate chain from an independent
machine. Allow inbound 443 only; 8787/8788 and the volume are private. Maintain
normal host patching, firewall, disk monitoring and restricted administrator access.

`Dockerfile.cloud` is an alternative build, with a non-root UID 10001 and no shell.
A container needs a durable writable volume, private config/key mounts owned by
that UID and a deliberate TLS topology. Do not mistake a loopback container port
for a reachable reverse-proxy service; use a shared network namespace/host-local
proxy, or configure application TLS for a non-loopback bind. Never run both the
systemd service and container on the same state. Pin the built image digest.

Register the minimal GitHub OIDC broker and exact browser callback described in
the preview guide. Qualify `sub`, audience, client identity, PKCE, token refresh and
revocation with real clients before enabling MCP. An accepted discovery document
alone is insufficient. Only after those gates pass, raise `enrollmentLimit` to the
approved cohort, maximum 100, and restart gracefully. Zero rejects new accounts
while existing users can still sign in and operate their watches.

## Observe independently

An external observer must check public `/healthz` with the exact Host and TLS.
Ding must not be its own only outage detector. Privately scrape
`http://127.0.0.1:8788/metrics`: workspace count, failed workers, running watches,
pending deliveries, unhealthy entities, maximum scheduler lag and pending delivery
age. These aggregate metrics contain no watch IDs, URLs, payloads or credentials.

Initial alert thresholds to validate during the pilot: public health unavailable
for two checks; any failed worker; scheduler lag over 30 seconds for two minutes;
delivery age over five minutes; filesystem less than 20% free; backup older than
26 hours; increasing authentication failures from a redacted proxy/provider
counter. Monitor process RSS/CPU, open descriptors and physical SQLite/WAL bytes
in addition to Go heap and per-workspace logical quotas. Failed/slow customer
endpoints are not themselves proof that the cloud worker is down.

For a worker failure: close enrollment if capacity/cost is implicated; retain the
volume; inspect bounded constant-message service logs and aggregate metrics; stop
and verify the old process before restarting. Do not copy live WAL files or start
a second worker on an old snapshot. Credential errors require rebinding, not
relaxing network restrictions. Delivery retries remain at-least-once; receivers
should deduplicate using stable delivery IDs. Never promise exactly-once webhooks.

## Back up and restore

Generate a separate age identity with `ding-cloud backup-keygen --output PRIVATE_FILE`.
The command prints only its public recipient. Store the identity off the worker;
place the recipient at `/etc/ding-cloud/backup-recipient`. Create a dedicated
`/var/backups/ding-cloud` owned by the service user. Install `backup.sh` under
`/opt/ding-cloud` and the supplied backup service/timer. Review before enabling it.

The supplied daily 03:00 UTC job deliberately stops the worker, obtains fenced
SQLite snapshots, encrypts a bounded archive using age, restarts the service and
expires its own snapshots after seven days. This creates a monitoring gap. A
single-volume prototype must disclose that maintenance limitation; it is not HA.
Replicate encrypted completed snapshots off-host with a bucket lifecycle matching
the retention promise. The job's seven-day local expiry is not a remote deletion
policy. Alert on backup/replication failure and record actual duration/bytes.

Snapshots include workspace state and encrypted cloud credentials. They exclude
the master data key, age identity, TLS keys and OIDC configuration: all require
separate protected recovery. Preserve deletion requests from the retention window
in a separately protected operator register so an old backup cannot undo them.

Restore into a **new, empty absolute directory**, with the old worker stopped:

```sh
ding-cloud restore --data-dir /NEW/restore --input /BACKUP/snapshot.age \
  --identity-file /PRIVATE/backup-identity
```

Restore authenticates the whole encrypted stream, checks inventory/SQLite integrity,
pauses watches, cancels stale pending deliveries and revokes restored browser,
device and MCP sessions. A durable quarantine marker prevents startup. Retained
history is evidence; canceled deliveries must not silently replay. Before release:

1. Prove the former worker cannot execute or deliver, including another disk clone.
2. Recover the original master key separately; test decrypting a synthetic credential.
3. Reapply deletion requests made since this snapshot and inspect transfer holds.
   Use a quarantined copy for inspection; do not expose restored accounts publicly.
4. Run `ding-cloud release-restore --data-dir /NEW/restore --confirm-other-runners-stopped`.
   This removes the quarantine and held transfer fences; it does not resume watches.
5. Point the stopped service at the restored directory with correct ownership,
   start with new enrollment closed, sign in afresh, rebind model connections and
   deliberately resume selected watches after review. Validate an actual check and
   remote notification before reopening enrollment.

Measure backup age (RPO), full restore time (RTO), decryption, isolated workspace
access and reactivation on a fresh host. The short automated recovery test is not a
production RPO/RTO promise. Repeat the drill before every storage/schema change.

## Keep the free beta bounded

The engine enforces watch/cadence/response/delivery/storage quotas and durable UTC
monthly outbound budgets; failures count toward attempts. Per-host limits and a
shared request bound prevent unbounded concurrency. These controls do not pay the
provider invoice. Keep cost alerts outside Ding and assign an operational owner.

Record fixed compute, volume/backup storage, traffic, identity, observability and
support separately. Calculate cost per **retained useful watch**, not signup.
Obtain current provider quotes before choosing a deployment; the roadmap's
$200–500/month envelope is an unapproved planning hypothesis. At the approved
threshold close new enrollment, investigate abuse and explain any workspace limit
visibly; do not silently mark stopped checks healthy. Expand only after the
72-hour workload gate and a meaningful retention window, with measured capacity
headroom, actual costs and an explicit new budget.
