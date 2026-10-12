# WorkOS AuthKit for Ding Cloud

Ding uses WorkOS AuthKit for hosted sign-in and Connect for MCP authorization.
The customer installs Ding, signs in, approves watch access, and returns to
ChatGPT. The first authenticated tool call creates a personal workspace and
connection automatically. Customers do not paste client IDs or grant tokens.

## Provider decision

Use a publisher-registered, **third-party public OAuth application with PKCE**
for ChatGPT. Third-party status is important: WorkOS explicitly requests user
consent. The associated WorkOS organization represents the application publisher;
it is not a shared Ding workspace and customers should not be added to it.

Use a separate **first-party public OAuth application with PKCE** for the Ding
Console. Its only requested scope is `openid`, and its callback is
`https://cloud.ding.ing/auth/callback`. Keeping the browser and ChatGPT clients
separate prevents a Console login token from being accepted as ChatGPT consent.
No WorkOS API key or OAuth client secret is required by this configuration.

WorkOS supports CIMD/DCR, but dynamically registered clients receive only its
standard OIDC scopes by default. Custom default scopes require WorkOS support.
The provided-client mode gives us Ding's four scopes now, without adding a
customer setup step. Revisit CIMD when custom scope provisioning is available.

Sources: [MCP setup](https://workos.com/docs/authkit/mcp),
[OAuth applications](https://workos.com/docs/authkit/connect/oauth),
[token claims and scopes](https://workos.com/docs/authkit/connect/token-claims),
[OpenAI authentication](https://developers.openai.com/plugins/build/auth).

## Production configuration recorded 2026-10-12

Billing is active and both production applications have been created. These are
public identifiers, not credentials:

| Setting | Value |
| --- | --- |
| Environment | `environment_01M4Q2KQ38QHN4BDH8MYTBGEHK` (Production) |
| Issuer | `https://glamorous-flame-84.authkit.app` |
| ChatGPT application | `app_01M4Q5KG83ZQA2WTBKGDBTCAAV` |
| ChatGPT client | `client_01M4Q5KG82X482D034Z37TM8NT` |
| Publisher organization | `org_01M4Q5HDEMB8HDC0KZYMVCX18J` (Ding ChatGPT Integration) |
| Resource indicator | `https://cloud.ding.ing/mcp` (saved default) |
| Allowed custom scopes | `ding:inspect ding:preview ding:manage ding:retry` |
| Client authentication | Public, PKCE (`none`) |
| Browser application | `app_01M4Q5EF5RMBPDZW2KTNZ4SK1M` |
| Browser client | `client_01M4Q5EF5QTGS96RSM5F8EJ3F2` |
| Browser callback | `https://cloud.ding.ing/auth/callback` (saved default) |
| ChatGPT redirect URI | Awaiting the exact URI from ChatGPT's connection portal |

The production issuer's discovery responds with authorization, token and JWKS
endpoints, S256 and refresh support. It does not advertise RFC 9207 authorization
response issuer support; use the exact callback-ID-specific URI, not a guessed
fixed callback. DCR and CIMD remain disabled for this provided-client release.
The free WorkOS-hosted domain is sufficient; no custom-domain add-on was enabled.

The deployment profile is:

```json
{
  "issuer": "https://glamorous-flame-84.authkit.app",
  "browserClientId": "client_01M4Q5EF5QTGS96RSM5F8EJ3F2",
  "chatgptClientId": "client_01M4Q5KG82X482D034Z37TM8NT",
  "enrollmentLimit": 10
}
```

Production WorkOS configuration is complete except for the ChatGPT callback.
Railway still runs the earlier password-only revision until the maintenance
rollout is approved and completed. No live authorization or refresh has yet been
qualified. OpenAI's plugin portal blocks creation/upload until the publisher
completes individual or business identity verification. No verification was
submitted by the agent. After verification, create the plugin/connection, copy
its exact callback and domain challenge, then configure and test them. Ding now
supports `DING_OPENAI_APPS_CHALLENGE` for that exact plaintext challenge.

## Staging configuration recorded 2026-10-12

These are public identifiers, not credentials:

| Setting | Value |
| --- | --- |
| Environment | `environment_01M4Q2KNMBPC25N88QRVVN1MY4` (Staging) |
| Issuer | `https://soft-story-24-staging.authkit.app` |
| ChatGPT application | `app_01M4Q2T0BZVHEJDF913D3GJWKX` |
| ChatGPT client | `client_01M4Q2T0BZD69JQ6PJXWM3G1RA` |
| Resource indicator | `https://cloud.ding.ing/mcp` |
| Allowed custom scopes | `ding:inspect ding:preview ding:manage ding:retry` |
| Client authentication | Public, PKCE (`none`) |
| Browser application | `app_01M4Q3BG86BF9JJFCW2PRR6YK8` |
| Browser client | `client_01M4Q3BG857NKE823CRXF7VED6` |
| Browser callback | `https://cloud.ding.ing/auth/callback` |
| ChatGPT redirect URI | Still to be copied from the actual ChatGPT connection |

The staging issuer's discovery advertises S256, authorization code and refresh
flows, plus its public JWKS. It does **not** currently advertise
`authorization_response_iss_parameter_supported`. Therefore use the exact
callback-ID-specific redirect shown in ChatGPT's management page; do not assume
the older fixed redirect URI or add a wildcard. No production configuration has
been deployed, and a live ChatGPT authorization flow has not yet been qualified.

## Configure a deployment

1. Register the two public clients described above. Assign the ChatGPT client's
   four custom scopes in Connect. Standard scopes `openid` and `offline_access`
   are requested by Ding's cloud package for sign-in and refresh. Do not assign
   administrator, credential-management, command-execution, or transfer scopes.
2. Add the exact `/mcp` resource indicator in Connect configuration. Confirm the
   access token's `aud` matches it, `iss` equals the configured issuer, `client_id`
   matches ChatGPT, and `sid` is the approved application consent ID.
3. Set `DING_AUTHKIT_JSON` using the correct environment's values:

   ```json
   {
     "issuer": "https://YOUR-ENVIRONMENT.authkit.app",
     "browserClientId": "YOUR-CONSOLE-CLIENT",
     "chatgptClientId": "YOUR-CHATGPT-CLIENT",
     "enrollmentLimit": 10
   }
   ```

   The limit is the total workspace capacity, including existing workspaces.
   Zero closes new signup while preserving existing accounts. Start with a
   bounded private pilot. Do not mix this profile with `DING_OIDC_JSON`,
   `DING_MCP_JSON`, or `DING_MCP_BRIDGE_CLIENTS`.
4. Existing password-only deployments pin their login methods. In maintenance,
   take an encrypted backup and run `enable-oidc-login --config /run/ding/config.json`
   following [the Railway runbook](../../deploy/railway/README.md#enable-hosted-sign-in).
   This enables the configured provider while preserving existing password
   accounts; new identities enroll separately. No identity migration is needed.
   The separate `enable-oidc` operation is available for deliberate account linking.
5. Deploy reviewed code, verify discovery and `/mcp`, and test the browser login
   before allowing customers to connect. Production must use an activated
   production WorkOS environment and its own clients, not the staging IDs above.

## Authority and connection lifetime

Only explicitly configured AuthKit clients can provision accounts from tokens.
Ding requires a verified issuer, exact resource audience, user subject, client,
`ding:inspect`, and application consent ID. Machine-to-machine tokens and tokens
with an organization context are rejected. This first rollout supports personal
accounts with exactly one Ding workspace; organization and multi-workspace
onboarding need an explicit mapping before they can be offered.

The provider consent ID is hashed with issuer, subject and client. Its private
backing grant keeps a stable ID across refreshed tokens, preserving operation
receipts. Verified requests renew its thirty-day window and intersect scopes
with current membership. Disconnect leaves a durable tombstone: the same
consent can never recreate its grant, even from an unexpired token. A genuinely
new consent creates a new connection; no raw provider token is stored in Ding.

A required live qualification is WorkOS's reconnect behavior after disconnect.
WorkOS normally reuses existing consent. Merely obtaining another access token
is therefore insufficient after Ding disconnects it. WorkOS documents
[`DELETE /user_management/users/:user_id/authorized_applications/:application_id`](https://workos.com/docs/reference/authkit/user#delete-an-authorized-application)
to revoke a user's Connect authorization. Integrate this into disconnect using
the verified immutable user identity and a server-held WorkOS API key; no key has
been created or configured yet. Confirm the actual application identifier and
prove that subsequent authorization produces a new `sid`. Keep local revocation
effective even when the provider is unavailable, with a retry path for cleanup.
Until implemented and demonstrated, reconnect remains a launch blocker;
do not weaken tombstone checks to make an old token work again.

Also verify consent cancellation, account capacity, browser session reuse,
refresh after access-token expiry, insufficient scopes, membership revocation,
and the provider's consent display of all four custom permissions. The signed
HTTP tests exercise Ding's side; they are not proof of live provider behavior.
