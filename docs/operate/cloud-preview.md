# Ding Cloud source preview

Ding Cloud is optional. Local Ding, local MCP, and an always-on machine remain
account-free. The service described here is implemented in source; no public
Ding-operated endpoint, marketplace approval, uptime commitment, or budget is
implied. See the [release gates](../development/local-first-progress.md).

## Run a private qualification instance

Build with `go build -tags console,mcpui ./cmd/ding-cloud` after building the two
web asset bundles. Supply an owner-only JSON file:

```json
{
  "publicURL": "https://ding.example.org",
  "listen": "127.0.0.1:8787",
  "dataDir": "/srv/ding/data",
  "keyFile": "/run/secrets/ding-data-key",
  "identity": {
    "issuer": "https://identity.example.org",
    "clientId": "registered-browser-client",
    "clientSecret": "registered-client-secret"
  },
  "mcp": {
    "issuer": "https://identity.example.org",
    "jwks_uri": "https://identity.example.org/oauth/v2/keys",
    "audience": "https://ding.example.org/mcp",
    "algorithm": "RS256"
  }
}
```

The key file contains exactly 32 random raw bytes, outside the data/backup volume.
Keep protected recovery copies separately. Losing this key loses encrypted source
credentials. Use a maintained OIDC broker configured with minimal GitHub identity,
no repository, organization, or private-email scopes. Register the exact callback
`https://ding.example.org/auth/callback`; enable authorization code with PKCE S256.
Ding binds workspaces to the broker's immutable issuer/subject, so the browser and
MCP clients must receive the same stable subject for the same developer.

The reverse proxy must preserve the exact public Host and serve HTTPS. The
loopback listener trusts no forwarded identity headers. Non-loopback listeners
require configured `tlsCert` and `tlsKey`. One process owns the persistent volume;
there is no lease-based automatic failover. Start with at most 100 accounts.

Run `ding-cloud --config /private/cloud.json --check-config` for shape validation,
then omit `--check-config` to start. Shape validation does not prove identity,
DNS, TLS, key access, OAuth client behavior, or capacity.

## Connect a local installation

Use `ding cloud login --url https://ding.example.org`. Open the printed URL and
compare its code with the terminal before approving. The device receives a
separate seven-day credential in its private state directory. No local watch is
uploaded during sign-in. `ding cloud status` reads hosted usage; `ding cloud logout`
revokes only this computer's connection. Watches continue independently.

## Move one watch

1. Run `ding cloud move prepare WATCH` without `--yes`. Eligibility is checked
   locally before account access or upload. Private URLs, unsupported sources,
   faster cadence, oversized limits, active incidents/stateful windows, overdue
   observations, and pending deliveries require attention; nothing is silently
   rewritten.
2. Sign in only if choosing cloud. In the hosted Console, save the named source
   and destination credentials. Exports contain references, not secret values.
3. Run `ding cloud move prepare WATCH --yes`. This uploads only the selected
   definition, creates a paused target, performs a bounded source check, and sends
   labeled destination tests. It preserves local history and keeps local
   execution running. A private transfer journal is written before remote effects.
4. Receive the test, then run the printed
   `ding cloud move finish TRANSFER_ID --confirm-delivery`. Ding freezes the
   reviewed source revision/generation, confirms it is healthy and idle, pauses
   it durably, activates the target, and waits for an actual target observation.
   A short monitoring gap is possible. Evaluation starts fresh.
5. After any lost response, use `ding cloud move status TRANSFER_ID`. Reuse the
   same transfer; never resume a held source because the target is unreachable.
   `ding cloud move test TRANSFER_ID` reconciles the same test without resending.
   Add `--new-test` only when deliberately sending another labeled test after a
   failure, changed credential, expiry, or uncertain outcome.
6. `ding cloud move cancel TRANSFER_ID --yes` cancels only a target that never
   activated, then releases the source hold. It does not silently resume a source.

To return, rebind credentials locally, then use
`ding cloud move prepare WATCH --back --yes` on the original installation. Receive
the local test and finish the new transfer. The original history remains local;
cloud history is not relabeled as local history. A move into an unrelated watch
with the same ID is rejected. A canceled watch ID cannot be silently resurrected.

## Public MCP preview

The endpoint is `/mcp`; protected-resource discovery is
`/.well-known/oauth-protected-resource/mcp`. Tools and the embedded workspace use
the official Go SDK and the same reviewed-mutation receipts as local MCP.

The authorization server must issue short-lived signed access tokens with the
exact MCP resource audience, immutable `sub`, `scope`, and a verified `client_id`
or `azp`. Configure PKCE, client registration, refresh and revocation there;
Ding does not turn GitHub access tokens or device session tokens into MCP tokens.
A provider accepting a `resource` parameter without actually binding the token
is insufficient. Real ChatGPT/provider interoperability remains a release gate.

In Cloud System → Model connections, approve the registered client ID, its scopes,
expiry, and selected credential references. Values never enter tools. Disconnect
removes the binding immediately and revokes its internal grant; a still-valid JWT
cannot recreate it. The preview uses explicit client registration/consent and is
not yet a qualified zero-configuration public-marketplace install.

## Preview limits

Three public HTTP watches; five-minute minimum interval; three remote destinations;
seven days of ordinary detailed history; 64 MiB per workspace; 100 pending
deliveries; 30,000 check attempts, 1,000 delivery attempts and 128 MiB metered
application traffic per UTC calendar month. Failed attempts count. Outbound work
fails visibly after budgets are exhausted. Pending/active evidence can remain
pinned beyond ordinary retention. Reaching a cohort limit restricts enrollment,
not existing users' local capabilities.
