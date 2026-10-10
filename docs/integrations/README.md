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
