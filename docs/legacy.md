# Legacy runtime and migration

The old workload wrapper is available as
[v0.14.0](https://github.com/ding-labs/ding/releases/tag/v0.14.0). Its
[versioned documentation](https://github.com/ding-labs/ding/tree/v0.14.0/docs) and
[README](https://github.com/ding-labs/ding/blob/v0.14.0/README.md) describe `run`,
`serve`, CI outputs, guards, and snapshots. The
[codex/legacy-maintenance branch](https://github.com/ding-labs/ding/tree/codex/legacy-maintenance)
keeps the hardened implementation. These commands are not reinterpreted by the
new binary; they exit with installation/migration guidance.

Pin the installer explicitly when keeping the legacy runtime:

```sh
DING_VERSION=v0.14.0 INSTALL_DIR="$HOME/.local/bin" sh scripts/install.sh
```

Create the chosen directory first. The script downloads release checksums and
verifies the archive. On Windows, use the release archive for your architecture.
Keep a separate binary name/path when running old and new installations together.

## Convert a configuration

```sh
ding migrate --config old.yaml --out converted --json
```

The output directory must be new. It contains `report.json` and one valid manifest
per fully supported rule. Unsupported rules emit no manifest. Partial conversion
writes the report and supported files, then exits nonzero with
`partial_conversion`. Conversion never reads credentials, starts watches, imports
snapshots, or changes a running daemon.

Supported conversions retain numeric conditions, level/cooldown behavior, string
selectors, all-string-label entity identity, literal/simple-field messages, and
console/webhook/Slack/Discord destination references. Dynamic legacy label sets
are represented by a canonical string field `legacy_group`; numeric extra fields
do not affect identity. Each rule becomes a push watch. Producers must send their
JSON input to each appropriate authenticated watch endpoint. Server-level jq is
composed with a bounded legacy JSON projection. Prometheus text requires upstream
conversion. Auto-format configurations convert only their JSON path.

Whole `${ENV}` destination URLs become secret references. Literal/composed URLs
are replaced by a generated environment name and a binding instruction pointing
to the original config field. Secret values are not copied into reports or
manifests. Configure those bindings on the daemon before applying.

The report calls out unsupported run-lifetime aggregates, end-of-run hooks,
synthetic run events, HTTP guards, CI/Kubernetes outputs, unsupported providers,
and advanced/aggregate message templates. Rewrite those rules explicitly or keep
them on the legacy runtime. One unsupported component prevents the entire rule
from being emitted.

All output requires review because the runtime contract changes:

- New watches start without legacy cooldown/baseline state. No snapshot import.
- Time is accepted observation time. Provider timestamps do not drive windows.
- Windows exclude the exact lower boundary, unlike legacy inclusive windows.
- Full sample budgets become unknown instead of silently truncating evidence.
- Delivery payloads are versioned, persistent and at least once; downstream
  consumers may need updates. Only firing events are targeted by conversion, so
  new recovery events do not introduce extra notifications implicitly.
- Authentication, port, storage, logs and resource flags use the new daemon model.

Validate and replay each manifest, dry-run apply, inspect state reset/permissions,
then apply when ready. Conversion does not prove your producer's timestamps,
labels, data rate or downstream payload handling are compatible.

## Evidence and rollback

Before deleting the legacy runtime, commit `74801ba` ran both engines against
17 captured fixtures: comparisons, five window functions, compound expressions,
label grouping, empty selectors, templates and cooldown boundaries. The exact
window-edge difference is a separate declared expected result. The fixtures
remain in `testdata/migration/parity.json`; the capture source is retained as
`legacy-oracle_test.go.txt`. The new tests run against these fixed expectations.

Reapply a saved watch definition to roll back configuration; compatibility rules
still decide whether state resets. For a binary/schema rollback, stop the daemon
and restore a verified compatible backup into a new private directory. Never run
an older binary on a newer store. Pending deliveries keep their original
immutable destination revision.

The proposed legacy maintenance window is critical reliability/security fixes
for 90 days after the first stable watch release. That stable release date has
not been set; no maintenance end date is implied by this preview.
