# Cloud MCP connections and qualification

The Ding server serves Streamable HTTP at `/mcp` when its OAuth authority is
configured. Password/browser sessions and CLI device tokens are separate
credentials and are never accepted as public MCP bearer tokens. Local-only MCP
continues to work without an account or network connection.

On October 11, 2026, read-only probes of `https://cloud.ding.ing` reported
password-only login. `/mcp` and `/.well-known/oauth-protected-resource/mcp`
returned 404. The repository contains a generic OIDC browser login with support
for a minimal GitHub broker, but no configured production issuer/client identity
was found in the checked-in deployment configuration. This implementation has
not been deployed or qualified against a real provider/model host.

## What production still needs

Configure a maintained authorization server with discovery, S256 PKCE, refresh
support, asymmetric JWT access tokens and a public JWKS endpoint. Ding requires
`iss`, a stable `sub`, expiry, the exact audience `https://cloud.ding.ing/mcp`,
`scope`, and `client_id` (or `azp`). Browser and MCP clients must receive the same
subject for one person; client-specific pairwise subjects cannot identify the
same Ding account. Never use mutable email matching.

Register these distinct clients with the chosen provider:

- Browser login: confidential client and exact callback
  `https://cloud.ding.ing/auth/callback`.
- Each supported model host: that host's actual OAuth client and callback,
  requesting only needed `ding:inspect`, `ding:preview`, `ding:manage` and
  `ding:retry` scopes. Register/configure these with the provider; inventing a
  client ID in Ding Console does not register it.
- Optional native Ding bridge: a separate public client with authorization-code
  and refresh grants and exact loopback callback `http://127.0.0.1:7678/callback`
  (or the explicitly chosen port). Only this client should request `ding:move`.
  Do not configure model hosts using this native client registration.

In addition to OAuth authorization, the user approves each client's workspace
binding in Ding Console. Effective permissions intersect the token's scopes,
current membership and unexpired grant. Removing consent blocks new requests
from a still-valid JWT. Owners alone can delegate moves, and only to native
bridge client IDs explicitly allowed by the operator.

## Enable the existing Railway account deliberately

Railway still defaults to password-only login and closed enrollment. New optional
variables are preserved by IaC and written to private runtime configuration:

| Variable | Meaning |
| --- | --- |
| `DING_OIDC_JSON` | Private JSON `{ "issuer": "https://identity.example.org", "clientId": "browser-client", "clientSecret": "..." }`; enables password **and** OIDC login |
| `DING_MCP_JSON` | JSON `{ "issuer": "https://identity.example.org", "jwks_uri": "https://identity.example.org/keys", "audience": "https://cloud.ding.ing/mcp", "algorithm": "RS256" }` |
| `DING_MCP_BRIDGE_CLIENTS` | Optional comma-separated native client IDs allowed to receive move consent |

Keep secret values in Railway's protected variables. These inputs are removed
from the serving process's initial environment after private runtime files are
written. No variable opens public enrollment or relaxes cloud execution policy.
An incomplete or mismatched authority fails startup.

Existing state pins its login methods. Ordinary deployment cannot silently add
OIDC. Follow the [Railway maintenance/backup runbook](../../deploy/railway/README.md),
set the new variables while in maintenance, and prepare an owner-only JSON file
outside the volume with the existing Ding user ID and the subject verified in
the provider's administration interface:

```json
{"user":"existing-ding-user-id","issuer":"https://identity.example.org","subject":"verified-immutable-subject"}
```

Run the stopped-state operator command as UID 10001:

```sh
/ding-cloud enable-oidc --config /run/ding/config.json \
  --identity-link-file /run/ding/identity-link.json
```

This atomically links that identity, pins the issuer, records an audit entry and
enables the password+OIDC profile. It preserves the account, memberships and
workspace, keeps enrollment closed, and rejects identity reassignment. The state
lock prevents it from running alongside the server. Repeat deliberately for
additional existing users. Remove the temporary link file after use. Return to
serve mode and qualify OIDC sign-in before approving model-client bindings.
No production state was changed by this implementation.

## Connect directly or through the local bridge

For hosts supporting remote MCP, use `https://cloud.ding.ing/mcp` as a separate
connection once enabled. `ding_list_workspaces` returns only approved, usable
workspaces. Supply `workspace_id` when there is more than one; it is resolved on
every request. Conflicting headers/arguments fail. Results identify the instance,
workspace and execution location.

For a combined adapter, approve a separate native client in Console, then run:

```sh
ding mcp cloud connect --profile /private/ding-cloud.json \
  --issuer https://identity.example.org --client-id registered-native-client \
  --workspace WORKSPACE_ID \
  --scopes ding:inspect,ding:preview,ding:manage,ding:retry

ding mcp cloud doctor --profile /private/ding-cloud.json
ding mcp serve --config /private/ding-local.json \
  --cloud-profile /private/ding-cloud.json
```

Sign-in uses the SDK's discovery, issuer binding, state and PKCE validation. The
profile is saved only after workspace consent and instance identity are verified.
Repeating `connect` refreshes sign-in for the same pinned identity; use a different
profile for a different target. Profile access and token rotation are serialized
across processes, and refreshed credentials are saved before use. Serving never
opens interactive sign-in. A missing/expired/unreachable cloud connection returns
an error; it never falls back to local or reuses CLI/admin credentials.

The bridge exposes `ding_list_connections`. Explicit `connection_id` is required
for ordinary tools. Its operation journal binds keys to the original request and
target before writes. Keep the private profile and its `.operations` directory
for recovery; no automatic journal expiry is performed. Identical watch IDs on
local and cloud remain independent. Local reads work while cloud is unavailable.

## Reviewed transfers

To enable transfer, pair locally with both `--manage --move` and the needed
`--allow-secret-ref` names. Approve move separately for the native bridge client
in the cloud Console and include `ding:move` when signing in. Existing manage
consent gains no move authority automatically. Secrets are rebound through
protected setup; only references and the selected definition are transferred.

1. Call `ding_get_transfer_preflight` on the source. It checks eligibility without
   connecting to the other instance or uploading. Review the manifest, cadence,
   destination revisions and changed network location.
2. After user authorization, call `ding_prepare_transfer` with the exact reviewed
   revision, generation and destination revisions, both connection IDs, an
   operation key and upload confirmation. The coordinator repeats local checks
   before remote access, checks target policy/credentials and journals both
   identities before preparation. Source keeps running; target is held paused.
3. Call `ding_test_transfer` with a stable key to send/reconcile the labeled test.
   A new key explicitly sends another test. Testing never pauses the source.
4. Only after the user confirms receipt and authorizes handoff, call
   `ding_finish_transfer`. The coordinator obtains real peer receipts from its
   authenticated sessions, fences source execution, then activates the target.
5. Call `ding_get_transfer_status` after an uncertain response and until first
   observation is confirmed. Activation can be committed while observation is
   still pending. Never interpret an unreachable peer as stopped or automatically
   resume a held source. `ding_cancel_transfer` only cancels a never-activated
   target and does not automatically resume a paused source.

Move-back uses a new reviewed transfer with the connections reversed. The
existing handoff state machine preserves original history and holds. Hosted-only
model clients can inspect eligibility/receipts but cannot coordinate a laptop.
The low-level peer-receipt tool is absent from ordinary model-client catalogs and
is available only to operator-approved native bridge clients and the bridge's
private in-memory adapter.

## Verification and live release gate

Local tests exercise the full hosted HTTP handler with a synthetic HTTPS issuer,
signed tokens, real tenant stores/workers, both supported protocol versions,
all original watch operations, policy limits, admission, membership changes,
multiple workspaces and revocation. HTTPS bridge tests cover refresh rotation,
restart and identity pinning. Real daemon transfer tests cover replay, delivery
confirmation, lost activation, restart, cancellation and move-back.

Before release, record real-provider and real-host results separately: OAuth
registration/login, workspace consent, both selected workspaces, expiry/refresh,
reconnect, immediate consent removal, a delivered cloud watch while the laptop is
off, and transfer recovery if offering the native bridge. Local tests and built
plugin archives do not establish marketplace acceptance or live interoperability.
See [verification](verification.md) and the [implementation plan](../development/mcp-cloud-plan.md).
