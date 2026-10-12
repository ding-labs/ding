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
No API key or OAuth client secret is required for sign-in or token verification.
Hosted consent revocation uses a server-only `DING_AUTHKIT_API_KEY`.

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
Production Email + Password and Magic Auth (one-time email codes) are enabled.
Password login uses WorkOS's Strong policy: at least ten characters, complexity
score at least three, and breached-password rejection. The hosted page exposes
email sign-in and signup. Google and GitHub are enabled as described below;
passkeys are not configured.

The deployment profile is:

```json
{
  "issuer": "https://glamorous-flame-84.authkit.app",
  "browserClientId": "client_01M4Q5EF5QTGS96RSM5F8EJ3F2",
  "chatgptClientId": "client_01M4Q5KG82X482D034Z37TM8NT",
  "enrollmentLimit": 10
}
```

Railway now runs `43226d8` with WorkOS-only sign-in. On 2026-10-12 the owner
requested a reset of the disposable development data. The active Ding state was
cleared in maintenance, then service resumed at 05:13:30 UTC with zero workspaces
or watches. WorkOS identities remain intact. A subsequent hosted login reused
the owner's WorkOS session and opened a fresh Personal workspace in the Console.
The owner confirmed that the earlier WorkOS signup worked.
Health, hosted-only auth configuration, rejection of native password endpoints,
and unauthenticated MCP challenges pass live checks. See
[qualification evidence](verification.md). ChatGPT authorization and refresh
remain unqualified, as do fresh Google and GitHub login round trips.

### Social providers

Both providers are enabled in production WorkOS. Their secrets are stored only
in WorkOS, and returning provider OAuth tokens is disabled. These registrations
authenticate people into the same hosted sign-in flow; they do not grant access
to Ding watches or replace ChatGPT's separate Connect consent.

| Provider | Public registration | Exact WorkOS callback | Identity scopes |
| --- | --- | --- | --- |
| GitHub | `ding-labs` app `3924844`, client `Ov23liWEu3uLd1gZMPtE` | `https://auth.workos.com/sso/oauth/github/klJ6WizLPYP4gE5OdzUewzj3x/callback` | `user:email` |
| Google | Project `fluent-fortress-511405-c7`, client `435877171447-uhb1o8r9cm8obidj79trqejbaumcdigg.apps.googleusercontent.com` | `https://auth.workos.com/sso/oauth/google/kYo3uHuJR1t7OLnmvT2zqZE7H/callback` | `userinfo.email`, `userinfo.profile` |

Google branding uses **Ding Cloud**, `ping@ding.ing` for support and developer
contact, and `https://ding.ing` as the homepage. The support Google account has
OAuth Config Editor access. The audience remains External / Testing: Google
disables publication until Branding is completed; public operator privacy and
terms links have not been supplied. Its documented
[basic identity scope exception](https://support.google.com/cloud/answer/15549945?hl=en)
allows these sign-ins without test-user registration, an unverified-app warning
or seven-day authorization expiry. Do not add other Google API scopes under that
assumption. Finish public branding separately before marketplace release.

To qualify each provider, open `https://cloud.ding.ing/auth/login` in a fresh
browser session, choose the provider, complete its identity consent and verify
the return to the intended Ding workspace. An existing WorkOS session can skip
provider selection; that proves session reuse, not a social login round trip.

The publisher completed OpenAI's individual verification submission, but its
dashboard still reports **Identity in review**. The plugin portal still blocks
creation/upload pending approval. After approval, create the plugin/connection, copy
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
the older fixed redirect URI or add a wildcard. Production uses the separate
configuration above; a live ChatGPT authorization flow is not yet qualified.

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
4. This profile uses hosted sign-in exclusively (`loginMethods: ["oidc"]`).
   Start with an empty Ding data directory; no instance owner/password bootstrap
   is needed. The first verified WorkOS login creates a personal workspace.
   Existing development data must be explicitly reset in maintenance before
   switching from a password profile; login methods are pinned in stored state.
   Resetting Ding data does not delete users from WorkOS.
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
to revoke a user's Connect authorization. Disconnect now resolves the stored,
verified subject at the pinned issuer and uses a server-held API key to find the
application by exact OAuth client ID. It revokes the local grant first and keeps
the disabled binding as a durable retry record until provider cleanup succeeds.
Console exposes Retry disconnect after failure. The same old consent remains
blocked. Provision `DING_AUTHKIT_API_KEY` privately and prove that subsequent
authorization produces a new `sid`; live reconnect remains a launch gate.

Use the separate [private ChatGPT test](chatgpt-private-test.md) for qualification.
`additionalChatgptClientIds` adds test clients without replacing the public client;
each client receives a separate grant and can be revoked or removed independently.

Also verify consent cancellation, account capacity, browser session reuse,
refresh after access-token expiry, insufficient scopes, membership revocation,
and the provider's consent display of all four custom permissions. The signed
HTTP tests exercise Ding's side; they are not proof of live provider behavior.
