# Local-first implementation progress

Started October 10, 2026. Source implementation of the [roadmap](local-first-roadmap.md).
Changes are committed by behavior in small commits. This branch is a source preview;
implemented code and passing automated checks do not establish a public release.

## Work ledger

| Area | Implemented in source | Remaining acceptance gate |
| --- | --- | --- |
| L0 installation and release channels | Private ownership records; embedded runtime/UI/MCP; native archive packagers; macOS package and per-user Windows installer; signed metadata and hash-pinned bootstrap | Real signing/notarization, Homebrew helper packaging, clean native installation matrix and public download checks. Legacy stable pointers stay unchanged. |
| L1 background startup | Login services for macOS/Linux/Windows; graceful lifecycle; Windows SCM host; dedicated-account boot templates; private logs and credential backends | Real logout/reboot/power-recovery tests, Windows/Linux task/service qualification and service-account credential provisioning |
| L2 status and repair | Offline CLI status; daemon identity/version; separate waiting/overdue/source/delivery health; cached updates; bounded logs; previewed narrow service repair | Broader native failure/repair matrix and usability. Ambiguous monitoring gaps remain explicitly unexplained. |
| L3 first useful watch | HTTP preview/review; first observation; OS notification test plus visible-confirmation gate; durable desktop outbox; disposable real scheduler demo; authenticated watch link and macOS click handler | Human-visible notifications/clicks, Linux/Windows click integration, ten-developer pilot and clean-machine five-minute target |
| L4 updates | Signed platform/channel/schema checks, capacity preflight, staged archives, backup, durable journal, equal-schema rollback/recovery, daily checks and explicit stable-only automatic maintenance | Signed packaged N→N+1 on each OS, native installer upgrade/uninstall and real power-loss qualification |
| L5 local MCP and self-hosting | Existing official Go SDK setup/pairing; independent daemon lifetime; local and dedicated-server guides; explicit OS boot templates | Real client/always-on-host matrix; no public marketplace compatibility inferred from stdio tests |
| M marketplace proof | Dated evidence, permitted-route questions, reviewer storyboard, architecture/privacy package and surface matrix | Publisher response/review. No inquiry or submission has been sent; desktop-only directory eligibility remains unresolved. |
| C0 topology and capacity | Single-host OS fencing, bounded per-tenant workers, shared admission and per-host limiting; reproducible synthetic benchmark | Representative sustained load, real RSS/CPU/network/backup measurements, current provider quote and approved budget |
| C1 identity and isolation | Maintained OIDC library, PKCE/nonce flow, minimal broker scopes, hashed browser/device sessions, per-workspace encrypted credentials, explicit enrollment cap/closure | Live GitHub broker setup and clean-account sign-in/revocation qualification |
| C2 hosted execution | Go engine per workspace; guarded public egress/DNS; durable quotas; hosted Console; remote delivery test; jitter; encrypted offline backups and quarantined restore | 72-hour representative soak, external observer, measured RPO/RTO, real receiver and laptop-off tests |
| C3 transfer and adoption | Offline eligibility before sign-in/upload; durable prepared target/source hold; credential-bound test proof; move, reconcile, cancel and move-back CLI; availability choices and opt-in pilot protocol | Real-device/client usability, every supported destination, retention observations and external-failure drills |
| C4 public MCP | Official Go SDK HTTP endpoint/UI; exact resource audience; verified issuer/subject/client; explicit scopes/credential references; revocable bindings and isolation tests | Real OAuth provider/ChatGPT PKCE/resource/refresh/revoke flow and publisher-approved zero-configuration onboarding |
| C5 beta operations | Non-root container and systemd/proxy templates; loopback aggregate metrics; retention; enrollment closure; export/deletion; backup job, restore/offline deletion commands; privacy and incident runbooks | Named operator, deployed monitoring/backup lifecycle, approved cost envelope, elapsed-time reliability and retained-use evidence |

## Verification recorded on this branch

- Full `go test -race -tags console,mcpui ./...` passed; `go vet` and module-tidiness
  checks passed. Focused tests cover credentials, egress, tenant boundaries, OAuth
  state, quotas, transfer retries/returns, deletion, restore and update interruption.
- Console unit tests passed; embedded Console/MCP UI builds and MCP UI tests passed.
  Initial Console JavaScript is about 153 KiB gzip, below its 250 KiB budget.
- Chromium and WebKit: 40 enabled browser tests passed on macOS. Firefox: 20
  enabled tests passed in the repository's isolated Linux ARM64 container. Six
  optional performance/visual capture cases were skipped across those runs.
  macOS Firefox could not create its temporary profile; no personal browser
  permissions were changed to work around it.
- Actual macOS ARM64 launchd install/start/restart/stop/uninstall passed with
  persisted watch state and cleanup. Synthetic native Keychain roundtrip,
  cross-installation isolation and cleanup passed. Both helper architectures build.
- Desktop binaries cross-compiled for Linux/macOS/Windows on AMD64 and ARM64;
  cloud binaries compiled for both Linux architectures. Windows service/DPAPI tests
  compile; only a native Windows run can qualify their operating-system behavior.
- The macOS archive and unsigned package build. The cloud container builds and
  runs its CLI as the configured non-root user. No signed public artifact or
  hosted deployment was produced by these checks.
- The refreshed [100-workspace synthetic run](cloud-topology.md) observed 100 healthy
  workspaces. It is an initial-load measurement, not a production capacity claim.

## Use and qualify the result

Start with [local setup](../operate/local-setup.md), [always-on operation](../operate/always-on.md)
and [native release engineering](../releases/local-packages.md). Optional hosted
execution has a [preview guide](../operate/cloud-preview.md),
[operator/recovery runbook](../operate/cloud-operations.md) and
[data-boundary document](../operate/cloud-privacy.md). The
[adoption pilot](adoption-pilot.md) measures local retention without required analytics.

Schema 5 preserves transfer ownership. Do not open that state with an older runtime
or restore a database merely to roll back an executable. Uncertain handoffs retain
a visible hold rather than authorize two alert producers. Cloud restores remain
quarantined/paused until the operator confirms old runners cannot execute.

## External release gates

Signing identities, live GitHub/OAuth registration, public-domain configuration,
publisher eligibility/review, physical reboot qualification, human-visible delivery,
real-device transfer and elapsed-time soak/retention results require external
evidence. Keep these gates open until they pass. No infrastructure was provisioned,
no provider budget was spent, no outreach was sent and no public listing was claimed.
