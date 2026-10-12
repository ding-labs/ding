# Integration qualification evidence

## Production social sign-in configuration — October 12, 2026

Both Google and GitHub are **Enabled** in the production WorkOS provider UI.
Provider secrets were transferred directly into WorkOS; they were not written
to the repository or this evidence record. Returning provider OAuth tokens is
disabled for both providers.

- GitHub OAuth app `3924844` belongs to `ding-labs`, with public client
  `Ov23liWEu3uLd1gZMPtE`, WorkOS's exact callback and only `user:email` access.
- Google project `fluent-fortress-511405-c7` has web client **Ding Cloud WorkOS
  sign-in**, public ID
  `435877171447-uhb1o8r9cm8obidj79trqejbaumcdigg.apps.googleusercontent.com`,
  WorkOS's exact callback, and only `userinfo.email` / `userinfo.profile` scopes.
  The owner-created Google identity `ping@ding.ing` has OAuth Config Editor access
  and is the saved support/developer contact. The homepage is `https://ding.ing`.
- Google audience is External / Testing. **Publish app remains disabled** with
  a request to complete Branding; public privacy and terms links are still empty.
  Google's [basic identity scope exception](https://support.google.com/cloud/answer/15549945?hl=en)
  permits these sign-ins without a test-user list, warning or seven-day expiry.
  This exception is not evidence of completed publication or brand verification.
- The live Ding `/auth/login` flow reused the owner's WorkOS session and returned
  to `/ui/watches` as owner of a fresh Personal workspace after the reset.
  Opening the standalone AuthKit preview also reused that session. Neither test
  exercised Google or GitHub; fresh social login round trips remain unqualified.

## WorkOS-only development reset — October 12, 2026

At the owner's explicit request, removed the disposable Ding application state
while retaining the WorkOS identity. Runtime revision `43226d8` disables instance
password login in the AuthKit profile and uses a provider-neutral sign-in label.

- Focused tests: `go test ./internal/railway ./internal/cloud` passed.
- Maintenance deployment `2c4eb306-ca36-41e4-930f-9a4d0496fc50` succeeded at
  05:12:26 UTC. `/healthz` reported stopped execution; process inspection showed
  only the maintenance launcher, with no Ding stores or workers running.
- Removed the exact active `/data/restored-20261011` directory and recreated it
  empty, private and owned by UID/GID 10001. No WorkOS users were deleted.
- Serve deployment `ef0ab54a-ba5c-4320-9f20-f5ff2eee389d` succeeded at 05:13:30 UTC.
  Health returned 200; auth configuration lists only hosted sign-in; both native
  password and activation endpoints return 404; unauthenticated MCP returns 401
  with protected-resource metadata. Railway reports one healthy replica.
- Private metrics reported zero workspaces, running watches and pending deliveries.
  The owner's next WorkOS sign-in creates a fresh workspace for new tests/soaks.
- Owner confirmed the original WorkOS signup completed before this reset.

Social provider setup and the subsequent fresh workspace login are recorded
above; neither requires a Ding runtime redeployment.

## Initial production WorkOS rollout — October 12, 2026

PR 37 is merged. Railway is pinned to implementation `a54c06a`, now included in
main by merge `50d57bb`. The production profile uses the public clients in the
[WorkOS runbook](workos-authkit.md), with total capacity of ten workspaces.

| Check | Live result |
| --- | --- |
| Maintenance | Deployment `9903d3d7-7368-4df8-9cce-062fae9a6b53` stopped execution at approximately 04:53:14 UTC; `/healthz` returned 503 and logs confirmed no stores/workers opened |
| Backup | `pre-authkit-20261012-0452.age`, 585,032 encrypted bytes, exported from the active state directory and downloaded privately; local quarantined restore passed before activation |
| Provider activation | Offline `enable-oidc-login` succeeded, preserving password accounts and enabling the pinned WorkOS issuer without identity linking |
| Resume | Deployment `4bb32d89-f142-46ce-8558-60786431fd9b` succeeded at approximately 04:54:38 UTC; one replica, no pending changes or reported failures |
| Health | `GET https://cloud.ding.ing/healthz` returned 200 and `ready` |
| Resource discovery | `/.well-known/oauth-protected-resource/mcp` returned the exact MCP resource, production WorkOS issuer and four Ding scopes |
| MCP authentication | Unauthenticated initialize returned 401 with `WWW-Authenticate` pointing to protected-resource metadata and `ding:inspect` |
| Console redirect | `/auth/login` returned the production WorkOS authorization URL with the Console client, exact callback, `openid`, and S256 PKCE |
| Hosted page | Enabled Email + Password with WorkOS's Strong policy and Magic Auth with owner approval; the live page now offers Continue with email and Sign up |
| Publisher verification | Owner submitted individual identity verification; OpenAI displayed `Identity in review` and continued to block plugin creation/upload |

The observed maintenance interval was approximately 84 seconds. The backup
verification restored into a separate local quarantine; production state was not
replaced. Neither backup credentials nor customer tokens are in this record.

The owner subsequently confirmed signup completed. ChatGPT user authorization,
refresh, provider-consent reset/reconnect, the actual ChatGPT callback, domain
verification and reviewer journeys remain unqualified.

## Cloud MCP implementation — October 11, 2026

Qualified locally on macOS ARM64 with the current main changes through `9341ccc`.
These results cover the implementation; no production OAuth configuration or
deployment was changed.

| Area | Result |
| --- | --- |
| Hosted boundary | Composed server tests use signed JWTs, a synthetic HTTPS issuer and real tenant stores; both supported protocol versions, watch lifecycle, embedded resource reads, workspace selection, scope/role/consent changes and revocation pass |
| Cloud policy | Preview/body limits agree across REST and MCP; workspace admission, usage/health reporting and effective permissions pass |
| Native bridge | HTTPS session tests verify refresh rotation, durable credentials, restart and pinned workspace/instance; target journals reject reused keys across connections |
| Reviewed transfers | Real daemon tests cover separate move consent, prepare/test/finish, lost activation, restart, first observation, cancellation and move-back; stale reviews do not contact the target |
| Account transition | Explicit existing-user OIDC linking preserves the workspace, pins the issuer and keeps enrollment closed; optional Railway configuration and secret-environment removal pass |
| UI and client | Seven MCP component tests, thirteen Console tests, both UI builds, eighteen-tool independent TypeScript client and two browser journeys pass; Console initial JavaScript is 163.3 KiB gzip against a 250 KiB budget |
| Repository | Normal and `console,mcpui` Go race suites, Go vet, Railway TypeScript check and Git whitespace checks pass |
| Cross-compilation | Windows AMD64 MCP executable and Linux AMD64 cloud server/Railway entrypoint build; this does not establish native Windows behavior |
| Packaged instructions | Both updated shared skills pass the skill-creator validator |

Read-only production probes returned password-only login and 404 for `/mcp` and
protected-resource discovery. A real issuer, browser/model/native client
registrations, existing-account identity link and deployment remain necessary.
Actual provider login/refresh, model-host consent/revocation and laptop-off cloud
delivery remain unqualified. Follow the [cloud rollout and acceptance
guide](cloud-mcp.md); synthetic test results are not evidence of those live flows.

## Earlier native adapter qualification

Verified October 10, 2026 on macOS ARM64. The adapter now uses the official MCP
Go SDK v1.8.0 and JWX v3.3.0. These are local development results, not marketplace
acceptance or testing inside actual ChatGPT/Claude.

| Area | Result |
| --- | --- |
| Portable contract | All 15 tool schemas/annotations and 16 structured/text result cases match captured FastMCP fixtures, including defaults, pagination, extra fields, and hostile evidence |
| Response failures | Malformed required data is rejected without exposing parser details; uncertain mutations retain same-key reconciliation and are never automatically retried |
| Daemon boundary | Real temporary daemon exercised through pairing, reads, preview/apply, receipt replay, lifecycle, evidence, and revocation; adapter serving imports no store/runtime package |
| Existing authorization | Concurrent retries, restart/expiry, principal isolation, stale reviews, generation guards, revoked access, denied commands/secrets, receipt-quota rollback, and delivery retry deduplication pass |
| Independent client | Official TypeScript client passes native stdio tools, UI-byte equality, both CLI entry points, mutation retries, revocation, and daemon survival after adapter close |
| HTTP | Both 2026-07-28 and 2025-11-25 formats pass, including concurrent identities, scope denial, binding rotation, discovery/challenges, and Host/Origin protection |
| JWT/JWKS | All six configured asymmetric algorithms pass; signature/claim/algorithm rejection, key rotation/removal, bounded unknown-key refresh, stale-cache failure, and redirect refusal pass |
| Pairing | A pairing created by the old packaged adapter works unchanged through both Go entry points; private files, existing-file refusal, bounded setup, nonce/Host/Origin/expiry checks, and concurrent pairing pass locally |
| Headless behavior | No UI resources or UI tool metadata are advertised without the bundle; unknown tools cannot reach the daemon; cancellation reaches the upstream request |
| UI components | Three tests pass: explicit apply, stable operation keys, expired previews, and hostile text |
| Browser UI | Two AppBridge journeys pass: restrictive CSP/light/dark/mobile/keyboard/offline behavior and actual native-Go-backed preview followed by explicit apply |
| Native package | Extracted macOS ARM64 ZIP is exercised by the independent TypeScript client; executable embeds the exact UI bundle and requires no installed Python/Node |
| Container | Scratch image contains the Go executable and CA certificates; read-only/no-network version smoke passes; missing OAuth config fails; real container verifies a synthetic HTTPS issuer and makes an authenticated HTTPS daemon call, rejecting missing/invalid tokens |
| Cross-compilation | MCP executable builds for Linux AMD64/ARM64, macOS Intel/ARM64, Windows AMD64; Windows private-ACL tests compile |
| Repository checks | Module tidiness, Go formatting/vet, normal and `console,mcpui` race suites, TypeScript build, bundle budget, workflow YAML parsing, and Git whitespace checks |
| Workflow skills | Both shared skills passed the skill-creator validator during the integration foundation work |

`make test-mcp` builds and tests the UI, runs the relevant Go race suites, and
executes the independent client and browser journeys. Install Playwright Chromium
once before running it locally. Container qualification is reproducible with:

```sh
docker build -f integrations/mcp/Dockerfile -t ding-mcp:qualification .
DING_MCP_CONTAINER=ding-mcp:qualification go test ./internal/mcpserver \
  -run TestContainerTLSAndAuthentication -count=1 -v
```

The container test uses temporary synthetic credentials and its own certificate
authority. It tests TLS verification and bearer authentication in the built
artifact; real identity-provider login/registration/refresh is a separate gate.

## Local artifact comparison

Same macOS ARM64 machine, version 0.1.0 and the same offline React UI. The
reference is the earlier dependency-complete FastMCP/PyInstaller qualification
artifact. Results are approximate and specific to these local builds.

| Measurement | Previous packaged adapter | Native Go adapter |
| --- | ---: | ---: |
| Runtime directory disk use | 55.6 MiB | 13.3 MiB |
| Plugin ZIP | 28.8 MiB | 5.0 MiB |
| Initialize plus first tool listing, median of 10 sequential subprocess runs | 508 ms | 15 ms |
| `--version` peak resident memory, one `/usr/bin/time -lp` sample | 53.8 MiB | 13.3 MiB |

The startup comparison uses the same independent SDK, temporary daemon, and
pairing. It includes process startup, MCP initialization, and `tools/list`.
It is not a cold-cache or sustained-load benchmark; the memory sample is a CLI
startup measurement, not steady-state server memory. Build time was not measured
comparably. Node remains a frontend build/test dependency; Python is removed
from integration source, dependency locks, builds, tests, packaging, and runtime.

Browser screenshots are generated under `web/mcp-app/test-results/`. Native
artifacts are under `dist/go-mcp/`; assembled packages are under `dist/plugins/`.
Generated artifacts and the temporary old runtime reference are not committed.

The native CI matrix covers Linux AMD64/ARM64, macOS Intel/ARM64, and Windows
AMD64, including extracted-artifact execution. Those remote jobs have not run
in this local session. Cross-compilation does not establish native behavior,
including Windows ACL enforcement. Actual client installation, publisher review,
signing/notarization, real IdP login/refresh, sustained upgrade testing, and
marketplace publication remain [release gates](release.md). MCP Events/chat
wakeups and a full service installer/updater remain outside this port.
