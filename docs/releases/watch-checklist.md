# Watch release checklist

This checklist does not authorize or announce a stable watch release. The current
public release remains the hardened legacy v0.14.0. Use a new prerelease version
for a watch beta; choose the actual number/date at release time.

## Beta

- [x] Versioned compiler/schema, immutable definitions and JSON CLI/API contracts.
- [x] HTTP, command and authenticated push adapters; bounded jq projection.
- [x] Deterministic conditions, timers, retained evidence and offline replay.
- [x] Atomic state/events/outbox, retry policy, provider backoff and identity.
- [x] Concurrent lifecycle, revision fencing, quotas and evidence-safe retention.
- [x] Backup/restore, authoring skill and actionable source/delivery health.
- [x] Supported legacy conversion/report and pinned v0.14.0 installation path.
- [x] Native executable and command-process tests on all six advertised targets.

## Stable qualification

- [x] Fault matrix covers real filesystem errors and subprocess crash boundaries.
- [x] Removed legacy CLI gives explicit migration instructions.
- [x] Fresh native executable drill verifies apply, push, replay, restart, restore.
- [ ] Real 24-hour capacity fixture completes; record measured memory, latency,
  source cadence, rejection/retry counts, retained rows and disk growth.
- [x] Record runtime commit's six archive sizes, container size and CI run.
- [ ] Publish only capacity claims supported by the fixture, including its history
  setting, admission retries, machine, payload and delivery assumptions.
- [x] Verify schema compatibility/refusal and pre-upgrade backup preservation.

## Publishing

1. Check the selected commit's CI and qualification artifacts, including the
   actual elapsed soak time. A shorter smoke run cannot satisfy that gate.
2. Tag a deliberately selected version only after the applicable gate passes.
   `release.yml` builds the six archives, checksums and CA-enabled containers.
3. Install the downloaded archive on a fresh machine and repeat the quickstart.
   Verify the tag and checksum, `version --json`, HTTPS polling and backup restore.
4. Publish release notes with the migration report, rollback steps, known limits
   and measured sizes. Update README, website/package description and docs together
   so the latest installer and quickstart agree. Do not overwrite unrelated local
   website edits during this change.
5. Keep the legacy tag/artifacts and maintenance branch. Start the proposed
   90-day critical-fix maintenance window only on the first stable watch release;
   record concrete start/end dates then.

If schema rollback is needed, stop the daemon and restore a verified compatible
backup into a new private state directory. Configuration rollback is a normal
apply and does not rewrite already queued destination revisions. See
[legacy migration](../legacy.md) and the [store contract](../development/store-contract.md).
