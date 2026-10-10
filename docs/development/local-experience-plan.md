# Plan: batteries-included local Ding

Proposed October 10, 2026. Part of the [local-first roadmap](local-first-roadmap.md). Commands, interfaces, and packaging below are proposed unless explicitly marked existing.

## Intended experience

Install Ding, run `ding setup`, accept a clear background-startup choice, and create a watch for a real endpoint. See a successful observation and receive a test notification within five minutes, with no Ding account. Close the terminal and AI client; monitoring continues. After the next supported login or boot, Ding resumes automatically. `ding status` and the Console explain what is running, what was last checked, and where notifications go.

For a developer who already runs an always-on machine, setup also offers a headless path using the same binary and manifest format. Cloud is not part of local setup.

## Current gaps and code boundaries

| Current repository behavior | Planned work |
| --- | --- |
| `scripts/install.sh` selects GitHub's latest release; current stable artifacts are legacy | Publish and select a qualified watch-runtime channel deliberately; align installer, website, docs, and package metadata |
| `workers/install/index.js` references the old `zuchka/ding` repository | Use the canonical release source and verify the deployed download path during release qualification |
| GoReleaser embeds Console/MCP UI, builds binaries/containers, and publishes a brew formula | Add signing, notarization where applicable, service integration, upgrade tests, and clearly owned install paths |
| `internal/watchcli/runtime.go` starts a foreground daemon and owns a single state directory | Add setup/service orchestration around that daemon; retain the single-writer boundary |
| `ding doctor` requires a reachable daemon | Add an offline-capable status/diagnostic layer that can identify a stopped or broken service |
| MCP setup pairs with an existing daemon | Put daemon setup before pairing; plugin lifetime must not control monitoring lifetime |
| Console, webhook, Slack, and Discord destinations exist | Add native desktop notification delivery and a test-notification flow |
| Export and daemon-managed backups exist | Integrate recovery, update safety, and migration guidance; backups do not include every external credential |

Keep implementation in this monorepo. Proposed new boundaries are `internal/service` for OS lifecycle, `internal/install` for installation ownership/update metadata, and a first-run UI in `web/console`. Reuse watch compilation, review, activation, outbox, and API contracts rather than introducing a second evaluator or installer-owned database writer.

## L0 — release and installation contract

Define an installation record containing version/channel, artifact digest, installation owner, stable executable path, service identity, and state directory. Detect existing installs before changing anything. Preserve the current state path, explicitly handle legacy installations, and explain incompatible data/commands before migration. Avoid creating a second empty instance because of platform directory spelling or case differences.

The default downloadable binary must contain the daemon, CLI, Console, and local MCP entry point. No end-user build tools or separate asset fetch. The initial install paths are Homebrew and a signed/notarized macOS package, verified native archives on Linux, and a signed Windows installer. Add distro packages/winget after their native lifecycle is qualified; do not advertise support merely because a binary cross-compiles.

Use a user-writable standalone install location by default where practical. Request elevation only for a clearly identified system installation or boot service. Provide download-and-inspect instructions alongside the convenience installer. Checksums detect corruption; release signatures and platform signing establish authenticity. Verify the artifact before replacement and reject unexpected OS, architecture, or channel.

**Acceptance:** clean machines can install without Go, Node, Python, or jq; the advertised commands exist in the downloaded binary; legacy-to-watch migration is explicit; repeated installation preserves state; the website download, brew formula, package, and docs agree on the version. No change to stable-channel pointers until watch-runtime release qualification passes.

## L1 — setup and background startup

Proposed command surface:

```text
ding setup
ding setup --headless
ding status [--json]
ding service install|start|stop|restart|status|uninstall
ding update check|install
```

Keep the existing `ding daemon`, `ding ui`, `ding doctor`, and `ding mcp ...` commands. Avoid reusing a legacy command name with incompatible semantics. Interactive setup detects the platform, presents the chosen state location and startup behavior, installs one service after the user's choice, starts it, waits for authenticated readiness, and opens the Console. A documented noninteractive mode must declare its choices explicitly and return actionable errors.

| Platform | Default workstation behavior | Explicit always-on behavior | Qualification detail |
| --- | --- | --- | --- |
| macOS | User LaunchAgent starts at login | System LaunchDaemon under an appropriate dedicated account, installed with elevation | Login is different from boot. Validate signed helper/notification packaging and locked credential storage. FileVault may still require an unlock after a power loss. |
| Linux with systemd | User unit starts with the user session | Explicit user lingering or a system unit under a dedicated account | Explain login/logout behavior; do not silently enable lingering. Validate permissions and executable paths. |
| Windows | User-logon task or qualified user-session helper | Windows Service under a least-privileged service account | Test battery, idle, duration, and reboot policies; desktop notifications need a user-session bridge. |
| Other Linux / containers | Documented foreground binary or container with persistent volume | User's supervisor/container restart policy | Label the support boundary; verify ownership, shutdown, and persisted state. |

OS lifecycle references: [Apple launchd jobs](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html), [systemd user lingering](https://www.freedesktop.org/software/systemd/man/252/loginctl.html), [Windows services](https://learn.microsoft.com/en-us/windows/win32/services/about-services). Confirm current packaging requirements on each supported OS during implementation; the archived Apple guide is a lifecycle reference, not a complete modern signing recipe.

One installation has one service owner. A Homebrew service and Ding's standalone installer must not register competing daemons. Reuse a matching running instance or explain the conflict; never kill an unknown process to claim its port. Use a stable executable path, bounded restart backoff, graceful shutdown, the existing state lock, private files, bounded logs, and an explicit service environment. Shell-only PATH and environment variables cannot be assumed to survive logout or reboot.

Resolve destination/source secrets through a credential abstraction: desktop credential storage where available, or explicitly configured private files for headless services. Preserve existing environment-reference compatibility. Show locked/missing credentials as a setup or service issue; do not weaken storage permissions to work around them. Commands run as the selected non-root service user with explicit paths and grants.

**Acceptance:** close terminal and model app, restart the daemon, log out/in, and reboot on native systems; exactly one evaluator owns the expected state and resumes watches. Distinguish user-login readiness from unattended boot readiness in every claim. Uninstall removes only owned service registrations and installed files; state is retained unless the user explicitly chooses purge.

## L2 — clear status and repair

`ding status` must work when the API is down. Combine service-manager evidence, the installation record, and an authenticated instance probe. Report service installed/running/stopped/failed, startup mode, instance identity, version, state location, last successful acquisition, recent monitoring gaps, active/paused/waiting watches, pending/failed deliveries, and update availability. Provide stable JSON for scripts.

Do not equate a live PID or HTTP response with working monitoring. Keep runtime health, source health, and delivery health separate. A watch with no observations is waiting, not healthy. A gap without OS evidence is an unexplained gap, not a confidently diagnosed sleep event. Do not replay fictional successful checks for time asleep.

Show the same facts in the Console, with concrete actions: start service, inspect a failed watch, test a destination, unlock/update credentials, repair a service registration, or view bounded logs. Repair must be narrow and preview its effect; preserve state and grants. Exports of diagnostics redact secrets and sensitive source/destination details by default.

**Acceptance:** stopped daemon, port conflict, stale connection file, locked database, inaccessible secret, exhausted disk, failed destination, and clock/suspend interruption each produce a distinct actionable status. CLI and Console agree when the daemon is reachable.

## L3 — a useful first watch and a visible alert

The primary setup flow asks for an HTTP health endpoint the developer cares about. Offer a local development service and a public deployed service as examples; do not require a third-party account. Preview the exact condition, interval, failure/recovery counts, timeout, destination, and where the watch runs. A starting template can use 30-second checks, three consecutive failures, and two successful recoveries; compile against supported semantics and show those choices before activation.

Base the first template on the existing HTTP status watch. Explain matching/nonmatching observations separately from transport failures: a timeout emits source-health evidence and follows the source-error policy; it must not manufacture an HTTP status or silently count as a successful check. Include source-error/recovery delivery in the preview so an unavailable endpoint produces useful feedback.

Reuse the engine's compile/test/review/apply path. Show the first actual result, including an honest explanation of an unreachable endpoint. Native desktop notifications are the account-free default when a user session exists. Ask for OS permission at the moment notifications are enabled, and send a clearly labeled test. Keep the in-app activity record even if permission is denied. A headless setup instead verifies an existing supported webhook/Slack/Discord destination or explicitly chooses console-only delivery.

Add a native-notification destination through the durable outbox. Use a small signed user-session helper if required by the OS; the Go engine remains authoritative. Activation must not assume that an OS accepted notification was read by a person. Notification clicks should open the relevant local watch through the authenticated Console handoff, without putting long-lived credentials in a URL.

Provide a separate one-minute, labeled demonstration using a temporary localhost fixture that transitions healthy → failing → recovered. Clean up its fixture/watch after the demo. It proves the alert pipeline without breaking the user's service. It does not count as the user's first useful watch or as production activation.

**Acceptance:** in a ten-developer pilot, target at least eight people independently creating a real watch and receiving a test notification within five minutes of starting installation. Record failures and repeat after repair. Account creation, YAML authoring, a model subscription, and optional analytics must not be prerequisites.

## L4 — predictable updates and recovery

Check for updates at a bounded interval and show availability; allow offline use and disabling checks. Default to user-initiated installation. Offer opt-in automatic compatible stable updates with a maintenance window; servers can pin versions. Package-manager installations delegate updates to their owner rather than overwriting its files. A plugin's adapter update and the daemon's update are separate lifecycles with a published compatibility window.

For standalone installs: verify signed metadata and artifact, check version/schema compatibility and disk space, prepare a recoverable backup, drain or record in-flight work, stop the owned service, atomically replace the executable, restart, and verify authenticated readiness and watch recovery. Serialize concurrent updates and use the platform's supported replacement mechanism, including Windows executable-lock handling.

Never reopen a migrated database with an incompatible old binary. Roll back the executable automatically only when database compatibility permits it. Otherwise preserve the failed state and offer an explicit backup restore, explaining possible loss of post-backup observations/deliveries. Document separate credential recovery; a SQLite backup alone is not a full machine backup.

**Acceptance:** interrupted download, invalid signature, incompatible schema, disk full, crash between replacement and restart, and failed health checks preserve recoverability. Upgrade with an active incident and pending delivery, then verify evidence, outbox, revisions, and MCP grants. Test release N → N+1 and supported rollback with actual packaged binaries.

## L5 — local MCP and always-on operation

After local activation, offer connection to supported AI clients. Reuse the existing pairing flow and conservative scopes; show what information reaches the chosen model provider and what writes require approval. A connection action must preserve unrelated client configuration, back up edits, use stable executable paths, and keep secrets out of conversations and client JSON when a private credential reference suffices. Prefer the approved marketplace installation flow when available; manual configuration remains an explicit developer fallback.

Qualify each actual host/version with create, preview, approve, inspect, revoke, offline recovery, and uninstall. Headless tool results must remain useful without embedded UI. Uninstalling a model plugin must not delete watches or stop the daemon; revoke its grant through supported lifecycle behavior. Do not depend on unapproved plugin hooks to install the service.

Ship a tested always-on guide for a Mac mini and a Linux server: install, boot/login mode, service identity, credentials, persistent storage, backup/restore, destination test, updates/version pinning, and health checks. Container instructions use a persistent volume and explicit restart policy. Monitoring cannot detect total failure of its own host while that host is down; an optional independent heartbeat observer is a separate choice. Do not silently change sleep, security, or disk-encryption settings.

**Acceptance:** a developer can run Ding indefinitely without a Ding account, cloud prompt, or vendor-hosted control plane. A 24/7 machine keeps working after the model client closes and recovers after a qualified restart. Explain power, network, and login dependencies accurately.

## Implementation sequence and release gates

| Work item | Main areas | Depends on | Evidence required |
| --- | --- | --- | --- |
| L0 release/install contract | Installer worker/script, GoReleaser, product metadata, install docs | Existing runtime qualification | Correct downloaded artifact and legacy migration tests |
| L1 service manager and setup | CLI, new service package, platform installers | L0 | Native restart/login/boot tests; private state and secret handling |
| L2 status/repair | CLI, control read models, Console | L1 contract | Offline and failure-mode cases |
| L3 first watch/notifications | Console onboarding, destination/outbox adapter, signed helper | L1–L2 | Real endpoint + labeled demo + developer pilot |
| L4 updates | Install metadata, release signing, backup/service orchestration | L0–L2 | Native upgrade, interruption, compatibility, and recovery tests |
| L5 client/server onboarding | Integration setup/packages, service docs | L1–L4; M for public ChatGPT packaging | Real host tests and always-on machine guide |

Start with the macOS vertical slice, then Linux and Windows. The general local release requires the supported-platform matrix to pass, the existing uninterrupted 24-hour runtime soak and Console usability/accessibility gates to close, and availability documentation to match actual artifacts. Cross-compilation, a demo recording, or unit tests alone do not establish those outcomes. See [runtime qualification](qualification.md), [Console qualification](console-qualification.md), and [integration release gates](../integrations/release.md).

No cloud service, account system, billing, public relay, or marketplace exception is needed to complete L0–L4.
