# Ding Cloud ChatGPT marketplace launch plan

The release goal is: install Ding in the ChatGPT marketplace, complete browser
sign-in and access approval, and return to ChatGPT connected. Users should never
need a terminal, server URL, OAuth client ID, token, private file, or a separate
Console configuration step. ChatGPT controls when it displays Connect or opens
authentication; Ding must complete its setup within that browser journey.

This is the follow-up to [PR 36](https://github.com/ding-labs/ding/pull/36).
The implementation is pushed and locally qualified. Production OAuth, the
integrated onboarding flow and marketplace publication remain outstanding.
Local/cloud bridging and watch transfers are outside this launch plan.

Implementation update (2026-10-12): per-tool OAuth declarations and scope-upgrade
challenges are implemented and tested over the real hosted HTTP transport. The
`cloud-chatgpt` package mode fixes the cloud endpoint, requires public publisher
configuration, and packages cloud-specific onboarding plus five positive and
three negative review cases. See the [publication runbook](../integrations/chatgpt-cloud-publication.md).

Read-only production inspection found no OAuth/OIDC variables on Railway's
Ding Cloud service. It remains pinned to `c5d9213` on `codex/railway-cloud`; these
PR changes are not deployed. The provider account, browser consent continuation,
durable connected-app grants and actual ChatGPT qualification remain unresolved.
The review cases are authored test procedures, not completed live evidence.


## The customer journey

1. Find **Ding** in the marketplace and install it.
2. Start Connect when ChatGPT prompts. The browser shows Ding sign-in, reusing
   an existing session when possible.
3. Existing customers keep their account and watches. New eligible customers
   receive one account and a default workspace automatically.
4. Approve clearly described watch access. Select a workspace only when the
   account actually has more than one. Technical identifiers remain hidden.
5. Return automatically to ChatGPT. The plugin identifies the connected
   workspace and offers useful starting prompts.
6. Later conversations reuse the connection. Refresh happens in the background;
   reconnect is reserved for revoked access or genuinely expired authorization.

Installing connects the service; creating a watch still requires reviewing its
definition. Disconnecting removes ChatGPT access without deleting running watches.
“Connected” means ChatGPT reads and manages the authoritative cloud workspace.
There is no local daemon or copied watch database in this flow.

## What exists and what remains

| Area | Present in PR 36 | Launch work |
| --- | --- | --- |
| Remote endpoint | Hosted `/mcp`, OAuth verification and discovery | Configure and deploy the real authority |
| Watch tools | Cloud inspection, preview/apply, lifecycle, evidence, workspace selection | Qualify actual ChatGPT behavior |
| Consent | Browser-approved user/workspace/client grants | Integrate consent into sign-in; eliminate pasted client IDs |
| Accounts | Password accounts, generic OIDC, explicit existing-account linking | Automatic eligible-user onboarding and browser account linking |
| Connection lifetime | OAuth refresh support; Console grants default to seven days and cloud grants cap at thirty | Add durable, revocable connected-app consent |
| ChatGPT linking | Per-tool OAuth scopes and scope-upgrade challenges | Verify the real ChatGPT linking UI |
| Package | Fixed cloud endpoint, explicit OAuth client mode, cloud onboarding and review cases | Supply production client settings, public policy URLs and live review evidence |
| Publication | Build/verification tooling | Publisher/domain verification, review account, submission and approval |

The existing [operator runbook](../integrations/cloud-mcp.md) can enable a private
pilot. Its manual account/client setup is an operator bootstrap mechanism, not
the intended customer experience. Earlier rollout instructions must be updated
as the integrated flow replaces those steps.

## 1 Resolve the production identity configuration

First inspect existing Railway variable names, deployed configuration locations,
and any existing provider project. Checked-in code does not establish whether a
provider already exists outside the repository. Reuse a compatible setup.

Use a maintained authorization server with the existing Ding verifier. Before
choosing a new service, prove it supports the complete login-and-consent
continuation needed below. A social-login-only product is insufficient if it
cannot issue access tokens for Ding's MCP resource.

The compatibility check must establish discovery, S256 PKCE, refresh, exact
resource audience, JWKS-backed access tokens, and consistent issuer/subject/client
claims. Validate authorization-response issuer handling and use the exact callback
shown by the real ChatGPT connection. These are host requirements to verify
against the selected provider, not new protocol code to invent.
[OpenAI authentication requirements](https://developers.openai.com/plugins/build/auth).

For the first integration, use a publisher-registered ChatGPT OAuth client unless
the existing provider already supports CIMD well. A predefined client is configured
once by us; customers still only sign in. Record the chosen mode in the package.
Keep client secrets in the provider/OpenAI portal, never in the ZIP.
[OpenAI package authentication](https://developers.openai.com/plugins/build/plugins#configure-mcp-authentication).

**Deliverable:** one recorded issuer, production/test client configuration,
callback, token claim example with values redacted, and a demonstrated consent
continuation. This resolves the provider decision before dependent implementation.

“Continue with ChatGPT” could further simplify sign-in if Ding receives access,
but it is currently a limited partner trial and still requires Ding connector
authorization. Do not make launch depend on that program.
[Sign in with ChatGPT for plugins](https://developers.openai.com/siwc/chatgpt-plugin).

## 2 Complete account and workspace consent inside the browser flow

Build a small connection page using the existing account, membership and grant
services. The provider's supported consent hook or continuation must supply
authenticated transaction context. Bind it to the verified user, actual OAuth
client, requested scopes, selected workspace and approved return destination.
Do not create workspace consent merely because a bearer token is valid.

The page should show **Connect Ding to ChatGPT**, the account/workspace name,
and plain-language permissions. Start with the existing inspect, preview, manage
and retry capabilities, constrained by the user's role. Continue to require
review before each watch change. Move and administrator permissions stay absent.

On approval, persist the existing grant/binding and resume the provider flow so
ChatGPT receives its authorization response. Cancellation leaves no usable
connection. Retries and a closed browser must not duplicate accounts or grants.
Use an expiring, single-use transaction and allowlisted return URLs.

Bootstrap the current owner with the offline identity-link command. For later
existing customers, require proof of their existing Ding account in the browser
before linking an identity. Never merge accounts solely by matching an email.

For new customers, create the account/default workspace exactly once after the
verified identity passes admission policy. Keep the pilot invite-only. Public
marketplace launch requires deliberate eligibility, capacity and abuse controls;
leaving enrollment closed would make a public listing unusable for new customers.

**Acceptance:** a fresh eligible account and the existing owner can connect from
ChatGPT without opening Console or entering any technical identifier. Denial,
duplicate callbacks, existing-account conflicts and full capacity show a useful
browser result and return safely to ChatGPT.

**Primary code:** `internal/cloud/mcp_connections.go`, `internal/serverstate`,
the identity callback/continuation integration, and a focused Console connection
page. Extract/reuse consent services rather than bypassing their checks.

## 3 Make connection and reconnection work naturally in ChatGPT

Declare the OAuth scopes on each hosted tool and return the appropriate
`_meta["mcp/www_authenticate"]` signal when authentication or additional OAuth
permission is needed. Keep policy denial distinct from expired authentication.
Test the resulting linking prompt in ChatGPT, including the embedded UI.
[OpenAI tool-level authentication](https://developers.openai.com/plugins/build/auth#triggering-authentication-ui).

Replace the seven-day setup pattern with a dedicated durable, revocable cloud
connection record. Access tokens remain short-lived; token refresh must never
restore a disconnected grant or expand scopes. Retain immediate membership and
permission checks on every call. Scope increases require another explicit
approval. Existing local and temporary grant expiry rules need not change.

One workspace should connect without a picker. Multiple workspaces should use
the existing explicit selection; avoid ambient mutable target state. Show a clear
connection/workspace label, useful reconnect guidance and one Disconnect action.
Evaluate an authenticated profile tool only if needed for host account labeling.

**Acceptance:** after access-token expiry, a new chat can list and manage the
same workspace without reentering credentials. After Disconnect, retained tokens
cannot access it. A canceled reconnect cannot create a second account or broaden
permissions. This is tested through ChatGPT, not inferred from synthetic tests.

**Primary code:** hosted tool metadata in `internal/mcpserver`/`mcpcontract`,
cloud consent persistence and `web/console/src/pages/CloudModels.tsx`.

## 4 Package Ding Cloud as the marketplace product

Generate the remote ChatGPT package with the fixed endpoint
`https://cloud.ding.ing/mcp`. Update the manifest and cloud onboarding instructions
to describe installation/sign-in and useful watch workflows. Remove local setup
and self-hosted configuration from this package's customer journey.

Reuse `internal/pluginpackage` and the shared watch instructions, with a cloud
setup variant where needed. Declare the selected OAuth registration method and
scopes explicitly in generated `mcp.json`. Ship branding, support/privacy/terms
links and tested prompts. The portable package consists of the manifest, remote
MCP declaration, skills and assets; it needs no bundled local executable.
[OpenAI plugin packaging](https://developers.openai.com/plugins/build/plugins).

Authentication must complete even if no notification destination exists yet.
Reuse an existing authorized destination for the first watch. A new customer can
connect Slack/Discord/webhook delivery in protected browser setup when first
needed, with an automatic return to the conversation. Current cloud policy only
supports those external delivery types; do not promise built-in email or future
ChatGPT notifications. Installation and first-alert delivery are separate checks.

**Acceptance:** inspect the generated ZIP and install it in the supported test
flow. Every setup instruction matches the cloud browser journey; no localhost,
manual client IDs, customer-supplied endpoint, credentials or native binaries are
required. A clean account gets actionable first-watch destination guidance.

## 5 Deploy and qualify the real private pilot

After PR review and green CI, back up the Railway state, configure the chosen
OIDC/MCP authority, link the existing owner and deploy a pinned revision using
the maintenance runbook. Keep native bridge configuration disabled. Verify the
metadata document and that unauthenticated MCP requests challenge for OAuth.

Use ChatGPT's custom-MCP plugin flow to test the deployed endpoint before public
submission. Account/workspace policy may control access. Refresh the connection
metadata after changing advertised tools or auth.
[OpenAI connect and test guide](https://developers.openai.com/plugins/deploy/connect-chatgpt).

Record one real end-to-end journey for each:

- Existing owner and fresh invited customer: connect, inspect workspace, return
  to a new conversation, and reconnect with the same identity.
- Create a public HTTP watch through preview and explicit apply; observe a real
  notification while the laptop is off; inspect the event and pause the watch.
- Expired access token, denied consent, insufficient role, disconnected grant,
  wrong workspace and interrupted apply. No duplicate write or renewed access.
- Embedded view and text fallback; first-run destination setup; clear errors when
  quotas or signup capacity prevent an action.

**Exit:** the intended browser flow is demonstrated in actual ChatGPT and
recorded in `docs/integrations/verification.md`. A successful Inspector call or
Go test alone does not satisfy this milestone. Restore the last qualified app
revision/configuration if rollout fails; account-state changes need their own
reviewed recovery procedure rather than an assumed image-only rollback.

## 6 Submit and publish

Verify the Ding publisher and endpoint domain, upload the cloud ZIP, complete
the MCP scan, and supply a dedicated sample-data review account. Prepare five
positive cases, three negative cases, a walkthrough and release notes. Resolve
review findings, then publish after approval. Public launch also requires the
new-account admission flow from step 2 and sufficient hosted capacity.
[OpenAI submission requirements](https://developers.openai.com/plugins/deploy/submission).

After publication, validate the marketplace install itself from a clean account.
The custom-MCP pilot is an intermediate test, not the finished installation flow.
OpenAI checks hosted MCP updates separately from package metadata/skill updates;
package changes require a new ZIP. Preserve compatible tools and keep a release
record connecting package version, server revision and qualification evidence.
[OpenAI update behavior](https://developers.openai.com/plugins/deploy/submission).

## Focused delivery sequence

| Work item | Depends on | Completion evidence |
| --- | --- | --- |
| Provider and ChatGPT client setup | Existing deployment inventory | Real compatible flow and recorded settings |
| Browser account/consent continuation | Provider continuation contract | Fresh/existing users connect without Console setup |
| Durable cloud consent | Browser consent service | Refresh succeeds; revocation remains final |
| Hosted auth metadata and errors | Selected scopes | Real ChatGPT linking/relinking prompts |
| Cloud package and first-watch guidance | Fixed endpoint and client mode | Valid ZIP and clean test installation |
| Railway pilot qualification | Above changes reviewed and deployed | Actual ChatGPT journey with laptop-off delivery |
| Marketplace submission and launch | Pilot, publisher and public admission ready | Approved listing and clean marketplace install |

Keep these as small, reviewable changes. The immediate next step is the provider
compatibility/consent check, followed by the browser connection flow. Further
local/cloud transfer work, native installers and ChatGPT wakeups are not on this
critical path.
