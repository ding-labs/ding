# ChatGPT and Claude integration

Ding now includes a native adapter using the **official MCP Go SDK v1.8.0**,
an embedded MCP Apps interface, and native/remote plugin packaging in this
monorepo. The Go daemon still owns
watch evaluation, state, authorization, and notification delivery. No model API
key is needed to operate a local Ding watch.

**This is an implementation and qualification build. It is not yet published in
either official marketplace.** Public installation is gated by publisher access,
signing, real-host qualification, and platform support for local/self-hosted
connections. See [release qualification](release.md). Source checkout/manual
connections are development paths, not a substitute for that release requirement.

For hosted public HTTP watches, [create a Ding Cloud account](https://cloud.ding.ing/auth/login)
and use the [Cloud Console](https://cloud.ding.ing/ui/) in your browser.
Local and self-hosted plugin packages keep their configured endpoint.
Marketplace qualification remains separate from creating a Cloud account.

## Local native setup

The native Claude package contains a Go executable with its embedded interface,
workflow skills, and a **Ding Setup** launcher. Open the
launcher, select an existing running Ding daemon's state directory, and choose
permissions. The pairing window is local, expires after ten minutes, and never
shows a credential. Then enable/restart Ding in Claude Cowork or Claude Code.
Ordinary Claude desktop/browser chat does not execute a local plugin server.

Pairing grants inspect and preview by default. Management and notification retry
are explicit choices. Grants last 90 days by default (1–365 configurable), can
be revoked immediately, and apply to the whole connected instance. Per-watch or
per-destination access restrictions are not implemented in this version.

For development:

```sh
npm ci --prefix web/mcp-app
npm run build --prefix web/mcp-app
go build -tags mcpui -o ding ./cmd/ding
./ding daemon --state-dir ./ding-state
# In another terminal:
./ding mcp setup --state "$PWD/ding-state"
```

The equivalent unattended local pairing command is:

```sh
./ding mcp pair \
  --state "$PWD/ding-state" --manage --retry
./ding mcp doctor
./ding mcp serve
```

The same command tree is available as `ding mcp` and the standalone `ding-mcp`.
Existing pairing and HTTP config formats are preserved; no re-pairing or database
migration is required for this port.

`serve` defaults to stdio and writes only protocol traffic to stdout. Pairing and
doctor are separate commands. The adapter never starts a daemon, opens its
SQLite database, executes a shell, or stops watches when the host exits.

Private connection files live in the platform's user configuration directory
under `Ding/mcp.json` (on macOS: `~/Library/Application Support/Ding/mcp.json`).
Use `--config` to select another file. POSIX files must be owned by the current
user with no group/world permissions (new files use 0600). Windows creates a
protected ACL for the current user and SYSTEM, checks ownership, and rejects
broadly readable files and reparse points. Older Windows pairings with broad
inherited ACLs must have their permissions repaired locally; keep the same grant.
A new pairing refuses to overwrite an existing file.
Do not copy administrator credentials into MCP settings.

## Permissions and changes

| Scope | Operations |
| --- | --- |
| `inspect` | Capabilities, bounded watches/events/deliveries/destinations, evidence |
| `preview` | Compile, optional fixture evaluation, and exact configuration review |
| `manage` | Apply reviewed changes; pause, resume, permanently delete watches |
| `retry` | Requeue an eligible failed/canceled notification |

All grants include inspect. Management does not imply permission to run arbitrary
commands or use arbitrary environment secrets. `pair --allow-command-revision
HASH` permits one exact compiled command-watch revision. `--allow-secret-ref NAME`
permits manifests to reference that daemon environment variable, including use
in outbound requests. Grant these only to trusted integrations: allowing a secret
reference permits its use at destinations selected by the manifest. The guided
pairing page enables neither capability. Existing destinations can be referenced
without recreating their definitions.

List and revoke grants locally:

```sh
ding-mcp grants --state /absolute/ding-state
ding-mcp grants --state /absolute/ding-state --revoke GRANT_ID
```

Every integration write requires an operation key (generate a UUID once per
intended action). The daemon commits the result receipt and effect in the same
SQLite transaction. A retry with identical arguments returns the saved result;
the same key with different arguments/action is rejected. A lost response is an
uncertain outcome: inspect `ding_get_operation`, then reuse the same key if needed.
Missing a receipt at one instant does not prove the original request has stopped.

Previews are opaque, principal-bound, expire after 15 minutes, and retain the
exact manifest, instance identity, watch revisions/generations/status, and
destination revisions. A concurrent change invalidates apply. Lifecycle changes
require both the inspected revision and generation. A preview is not permission
from the user; the model and host still honor the actual request.

The embedded app shares Ding Console's theme and offers watches, preview/apply,
event evidence, and delivery history. It uses only the MCP host bridge, contains
all scripts/styles locally, renders source text as text, and works in light/dark
and narrow layouts. Text tool results remain available without UI support.

## Ding Cloud MCP

The cloud runtime includes an optional OAuth-protected `/mcp` endpoint using the
same watch tools and embedded views. See the [cloud source preview](../operate/cloud-preview.md#public-mcp-preview)
for configuration and explicit client consent. This is source support, not a
claim that a public endpoint or marketplace install is available.

Local pairing and `ding cloud login` do not connect the local MCP adapter to that
endpoint. Cloud usage, workspace discovery, and watch transfer are not yet MCP
tools. The [cloud MCP plan](../development/mcp-cloud-plan.md) records the current
gaps, policy corrections, and implementation sequence.

## Self-hosted HTTP

HTTP is opt-in and **cannot start without explicit OAuth configuration**. The
adapter uses the official SDK's bearer authentication and protected-resource
metadata with JWX signature/JWKS verification. The identity provider
owns OAuth authorization, PKCE, client registration, login, and token issuance.
There is no custom Ding OAuth server or Ding-operated relay. Your provider must
support the chosen host's supported registration/connection flow.

1. Pair a separate local Ding grant for each remote identity. Keep the resulting
   connection file private and readable by the MCP service user.
2. Register the host's OAuth client with your identity provider (or enable a
   compatible DCR/CIMD flow). Configure an explicit audience and scopes.
3. Save the following as a private 0600 HTTP configuration. Replace every example
   value; these domains are documentation examples, not deployed services.

```json
{
  "public_url": "https://ding.your-domain.example",
  "issuer": "https://identity.your-domain.example",
  "jwks_uri": "https://identity.your-domain.example/.well-known/jwks.json",
  "audience": "https://ding.your-domain.example/mcp",
  "algorithm": "RS256",
  "subjects": {
    "subject-from-your-identity-provider": "/absolute/private/alice-ding.json"
  }
}
```

```sh
ding-mcp serve --transport http --http-config /absolute/private/http.json
```

The service listens on `127.0.0.1:7677` by default. Put a TLS reverse proxy in
front of that listener, preserve the public Host header, and forward `/mcp` and
`/.well-known/oauth-protected-resource/mcp`. Limit request bodies to 12 MiB and
apply normal connection/rate limits at the proxy. Never proxy the daemon's admin
port as part of the MCP deployment. For containers, explicitly bind with `--host
0.0.0.0` on a private network and mount private configuration read-only under a
service-owned UID (the included container uses 10001).

JWTs must have the configured issuer, audience, valid signature/expiration, and
`ding:inspect` scope. Each tool also requires `ding:preview`, `ding:manage`, or
`ding:retry` as appropriate. Its `sub` claim must match an explicit subject binding.
The bound daemon grant is rechecked on every call. Remote subjects never share a
mutable “current connection.” The OAuth token is never forwarded to the daemon.
Only the configured HTTPS JWKS endpoint supplies keys; redirects and token-provided
key URLs are ignored. The key cache expires after five minutes and refreshes at
most once per 30 seconds, including unknown key IDs. Supported algorithms are
RS256/384/512, ES256/384, and EdDSA; existing `Ed25519` configuration is an alias
for EdDSA. Revoking a Ding grant takes effect independently of that key cache.

## Development and verification

```sh
make test-mcp
make mcp
go run ./cmd/package-integrations \
  --mode native-claude --target darwin-arm64 \
  --runtime dist/go-mcp/darwin-arm64/ding-mcp \
  --output dist/plugins/claude-darwin-arm64
```

Run the native build on each target OS/architecture. A platform matrix in
`.github/workflows/integrations.yml` builds and tests native artifacts; it does
not publish or claim signing. `make mcp` uses ordinary Go compilation with no
Python build or runtime dependency. Node is needed only to build/test the React
UI. Headless `go build` works without Node and omits UI advertisement; plugin
builds require `-tags mcpui`, and main releases use `-tags console,mcpui`. The
container builds and embeds its own UI. The UI remains under 1 MiB without CDN
requests. Run `npx playwright install chromium` in `web/mcp-app` once before
local browser tests. Cross-compilation alone does not qualify another platform.

For a provisioned self-hosted endpoint, `go run ./cmd/package-integrations --mode
remote-chatgpt` or `remote-claude` requires `--endpoint https://YOUR_HOST/mcp` and
an unused `--output` directory. Native package versions default to 0.1.0; set
`MCP_VERSION` for `make mcp` and the same `--version` for the assembler. It generates
the proper platform manifest, skills, assets, and MCP configuration. This describes a fixed endpoint, not a
universal marketplace solution for arbitrary customer instances.

The current adapter does not implement MCP Events subscriptions/chat wakeups,
automatic OS-service installation, updater/signing infrastructure, or an
operator-facing per-object permission editor. Ordinary Ding webhook/Slack/
Discord delivery remains the notification path. Those are explicit remaining
milestones in [the plan](../development/llm-integration-plan.md).

## Cloud and combined connections

See [Cloud MCP connections and qualification](cloud-mcp.md) for the configured
`/mcp` endpoint, Railway OAuth setup, existing-account linking, workspace consent,
and the optional local/cloud bridge. Capabilities report `ding.mcp/v1.4`, effective
permissions and execution limits. Cloud previews allow 64 KiB manifests/fixtures
inside a 1 MiB request; local/self-hosted limits remain 1 MiB/8 MiB with a 12 MiB
request cap. Cloud calls share workspace admission with the browser API.

Local and self-hosted adapters add three transfer-inspection tools and a typed
service starter and destination setup inspection to the original 15 watch tools. Hosted adapters also add workspace
discovery, cloud status, and an authenticated account profile.
The optional combined adapter adds explicit connection discovery and five reviewed
transfer actions. Provider configuration and actual host qualification are still
required before claiming the deployed service supports these flows.


## Stable account identification and service starters

Hosted `ding_get_profile` takes an empty object and returns the authenticated
Ding user's immutable `id`, plus `name` when available. It returns the profile
directly as structured content and matching JSON text, with
`_meta["openai/profile"]: true`; it does not use the ordinary Ding view envelope.
It requires inspect authority and active workspace consent but does not select a
workspace. Email-label changes, reauthorization, and multiple approved workspaces
do not change the account ID. The ID is not an email, a workspace, or a grant.
This does not link accounts, infer email aliases, or guarantee a particular host
label. Use `ding_list_workspaces` separately to select the execution workspace.

`ding_preview_service_watch` accepts a new watch ID, HTTP endpoint, and an exact
saved destination reference from `ding_list_destinations`. For example:

```json
{
  "watch_id": "api-health",
  "url": "https://service.example/health",
  "destination_ref": "ops-alerts",
  "expected_status": 204
}
```

`expected_status` is optional and accepts an integer from 100 through 599. When
present, a different status is failure; when omitted, HTTP 5xx is failure. The
starter checks every five minutes in Cloud or every 30 seconds locally/self-hosted,
with a five-second timeout, three failing checks to fire, and two healthy checks
to recover. Source errors hold incident state and have separate error/recovery
events. Cloud retains its public-network and one-MiB response limits.

Supply `workspace_id` for an ambiguous hosted workspace selection; the combined
adapter requires `connection_id` as well. The starter rejects existing watch IDs,
unknown options, unsupported URLs, and missing destinations. Preview neither
fetches the service nor tests delivery nor creates a watch. It returns the
manifest, compiled descriptions, execution target, and the ordinary expiring
preview handle bound to the destination revision. Use `ding_apply_changes` with
that handle and a stable operation key under the user's authorization. Changed
revisions require a fresh preview; a repeated apply key reconciles one effect.

The REST equivalent is `POST /v1/integrations/preview/service`, using `id`, `url`,
`destinationRef`, and optional `expectedStatus` with the existing preview grant.
Advanced changes still use generic manifest preview. A typed preview does not
establish destination readiness or prove notification receipt. Use the protected
setup flow below for the guided [first-watch journey](../development/chatgpt-first-watch-plan.md).
These additions have automated qualification; production deployment and host
metadata/profile-label qualification must be recorded separately.


## Protected notification setup and receipt confirmation

Call `ding_get_destination_setup` with an existing `destination_ref`, or omit the
reference to obtain a Console setup link. It requires only current inspect access.
The response identifies the authenticated account and workspace and contains an
absolute `setupURL`; it never contains credential values or credential digests.
The local adapter and combined adapter support the same read contract, with the
combined adapter retaining explicit connection selection.

Open that link in the browser. The Console checks its account and workspace
against the link before showing setup actions. An OAuth sign-in preserves only
the validated setup coordinates; it does not sign in as a URL-specified account
or silently change workspace. A mismatch offers normal sign-out/sign-in and
workspace selection. These coordinates are not authorization tokens.

The protected page can reuse a saved destination or create a destination using
an existing credential reference or a credential saved directly to the workspace.
It creates no watch. Send a clearly labeled test and confirm receipt in the
Console only after seeing it. Acceptance by the provider and user-confirmed
receipt remain separate states. Test keys survive reloads, a lost response can
be reconciled without sending again, and confirmation itself is idempotent.
An uncertain attempt can be replaced only through an explicit additional-test
action that warns the earlier notification may already have arrived.

| State | Meaning |
| --- | --- |
| `missing` | No reference selected, or that destination does not exist |
| `unavailable` | Destination configuration or its resolved credential cannot be read |
| `untested` | No bound test receipt for this account/workspace/destination |
| `pending` | An attempt was reserved; acceptance is still unknown |
| `failed` | The destination rejected the attempt |
| `accepted` | Provider/OS acceptance recorded; user receipt not confirmed |
| `confirmed` | User recorded receipt for the current configuration within the validity window |
| `stale` | The destination revision or resolved credential/header binding changed |
| `expired` | The test is outside the 24-hour confirmation window |

Confirmation expires **24 hours after the test**, not after the confirmation
click. Reconfirming does not extend that window. The durable no-resend receipt is
retained after expiry. The current destination revision, resolved credentials,
account, workspace, and test operation bind readiness; every read requires current
authority. Models cannot submit a confirmation argument or call a confirmation tool.

After returning to chat, refresh the destination list if needed, read readiness
for the chosen reference, and obtain a fresh service preview before authorized
activation. This onboarding workflow does not add a universal notification-test
gate to advanced generic apply. The complete packaged assistant workflow and real
ChatGPT qualification remain separate milestones.

Browser APIs: `GET /v1/onboarding/destination-status?destination=ID`,
`POST /v1/onboarding/destination`, and
`POST /v1/onboarding/destination-confirm`. Creation and confirmation require an
interactive destination administrator (or the local owner) and an exact account/
workspace match. MCP uses only `GET /v1/integrations/destination-status`, under the
existing inspect grant. Capabilities report `ding.mcp/v1.4`.
