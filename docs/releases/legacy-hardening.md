# Legacy hardening release: v0.14.0

This release keeps the existing `run` and `serve` product while fixing the
reliability defects found during the watch-runtime audit. It is the supported
legacy baseline for the forthcoming persistent-watch migration.

Upgrade changes:

- The daemon binds `127.0.0.1` by default. All HTTP routes except `/health`
  require a bearer token. Ingestion and administration use separate tokens.
  Local defaults generate a private `ding/legacy-tokens.json` under the OS user
  configuration directory. `server.token_file` overrides the location. On
  Windows, protect this directory with the user profile's ACLs. Remote/container
  binding requires an explicit IP and both tokens. See the HTTP API reference.
- Configuration rejects unknown fields, duplicate rules, invalid URLs, and
  negative bounds. Environment expansion applies to decoded scalar values;
  environment values cannot add YAML structure. YAML aliases are rejected.
- Snapshot format v2 includes collision-safe identities and rule compatibility
  metadata. Preserve old snapshots as backups and explicitly reset unsupported
  snapshots. Old ambiguous identities cannot be losslessly recovered. Startup
  fails instead of silently overwriting incompatible data.
- Reload waits for current evaluation/dispatch and transfers compatible live
  state. Removed or incompatible rule state resets with a diagnostic. Listening
  address and credential changes require restart.
- HTTP 408, 429, and 5xx retry. Retry-After is honored; rejected deliveries no
  longer count as successes. Redirects are rejected and provider acknowledgments
  are checked. Drain deadlines report unfinished deliveries. Queues are still
  memory-only; a crash or expired shutdown deadline can lose pending delivery.
- Idle state is reclaimed with configurable cardinality bounds. Active windows,
  cooldowns, and run-lifetime state are retained. Ingestion reports quota errors.
- `validate`, `test-rule`, and dry-run compilation start no delivery resources.
  Live HTTP guards are explicitly unsupported in replay/dry-run. jq has time,
  output-count, and output-byte limits. Timestamps reject malformed values.
- Both images include trusted CA roots. Release images select the correct target
  architecture. Homebrew metadata uses Apache-2.0 and tests `ding version`.

The watch runtime is not part of this release. Existing configuration conversion
and the new execution/durability model follow in subsequent migration steps.
