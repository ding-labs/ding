# Ding Cloud ChatGPT marketplace launch plan

The release goal is: install Ding in the ChatGPT marketplace, complete browser
sign-in and access approval, and return to ChatGPT connected. Users should never
need a terminal, server URL, OAuth client ID, token, private file, or a separate
Console configuration step. ChatGPT controls when it displays Connect or opens
authentication; Ding must complete its setup within that browser journey.

This is the follow-up to [PR 36](https://github.com/ding-labs/ding/pull/36).
The implementation is pushed and locally qualified. Live OAuth/onboarding
qualification and marketplace publication remain outstanding.
Local/cloud bridging and watch transfers are outside this launch plan.

Implementation update (2026-10-12): per-tool OAuth declarations and scope-upgrade
challenges are implemented and tested over the real hosted HTTP transport. The
`cloud-chatgpt` package mode fixes the cloud endpoint, requires public publisher
configuration, and packages cloud-specific onboarding plus five positive and
three negative review cases. See the [publication runbook](../integrations/chatgpt-cloud-publication.md).

WorkOS AuthKit with Connect is selected. Production and staging ChatGPT/Console
public PKCE clients, four Ding scopes, and the MCP resource indicator are
configured. Production billing is active. Automatic personal
workspace enrollment and renewable provider-backed grants are implemented and
locally tested. The [WorkOS runbook](../integrations/workos-authkit.md) records
public configuration and exact deployment steps.

Railway runs `a54c06a` with production WorkOS as of 2026-10-12 04:54 UTC.
Health, discovery, the MCP authentication challenge and the Console's PKCE
redirect pass live checks after backup and offline sign-in activation.
Authenticated browser/ChatGPT qualification, ChatGPT's exact callback,
live reconnect semantics, and marketplace qualification remain outstanding.
The review cases are authored procedures, not completed live evidence.
The owner submitted OpenAI publisher identity verification; it remains in review
and the portal still blocks plugin creation/upload. Domain-challenge serving is
implemented and awaits the token from the portal. Public operator privacy,
terms and support pages, review credentials, fixtures and walkthrough remain
submission prerequisites.

## The customer journey

1. Find **Ding** in the marketplace and install it.
2. Start Connect when ChatGPT prompts. The browser shows Ding sign-in, reusing
   an existing session when possible.
3. Previously linked identities keep their account and watches. New eligible customers
   receive one account and a default workspace automatically.
4. Approve clearly described watch access in WorkOS. The initial release supports
   personal accounts with one workspace. Technical identifiers remain hidden.
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
| Remote endpoint | Live `/mcp`, production OAuth verification and discovery | Qualify authenticated client requests |
| Watch tools | Cloud inspection, preview/apply, lifecycle, evidence, workspace selection | Qualify actual ChatGPT behavior |
| Consent | WorkOS-hosted approval; automatic provider-backed grants | Prove live consent and reconnect behavior |
| Accounts | Production configured for bounded personal-workspace enrollment | Qualify Console sign-in and fresh enrollment |
| Connection lifetime | Provider-backed grants renew and preserve revocation | Qualify actual refresh and consent reset |
| ChatGPT linking | Per-tool OAuth scopes and scope-upgrade challenges | Verify the real ChatGPT linking UI |
| Package | Fixed cloud endpoint, explicit OAuth client mode, cloud onboarding and review cases | Supply production client settings, public policy URLs and live review evidence |
| Publication | Build/verification tooling | Publisher/domain verification, review account, submission and approval |

The existing [operator runbook](../integrations/cloud-mcp.md) can enable a private
pilot. Its manual account/client setup is an operator bootstrap mechanism, not
the intended customer experience. Earlier rollout instructions must be updated
as the integrated flow replaces those steps.

## 1 Finish the selected WorkOS deployment

Follow the [WorkOS runbook](../integrations/workos-authkit.md). Use separate
public PKCE clients for ChatGPT and the Console, configure the exact MCP resource,
and use the callback URI from ChatGPT's management page. Production configuration
must use a production WorkOS environment. The existing Ding verifier checks JWKS,
issuer, audience, expiration, subject, client and scopes.

Use the provided-client registration mode for this release. WorkOS's default
CIMD/DCR scopes do not include custom Ding permissions without support setup.
This is publisher configuration; customers still only sign in and approve.

**Acceptance:** record real authorization and refresh results, redacted token
claims, and the exact environment/client/callback configuration. Public metadata
alone does not prove the completed flow.

## 2 Qualify hosted sign-in and automatic account creation

WorkOS provides the browser sign-in and explicit third-party consent page.
No custom Ding OAuth server or duplicated consent screen is planned. Its verified
application-consent claim materializes one personal Ding workspace connection,
limited by provider scopes and current Ding membership. Enrollment requires the
explicit deployment capacity; existing disabled identities cannot enroll again.

Implemented signed-HTTP tests cover fresh signup, reuse, scope narrowing,
revocation, new-consent reconnection, invalid token claims and closed enrollment.
The same consent cannot restore a disconnected grant. Test the corresponding
real WorkOS and ChatGPT journey before publication, especially how the provider
revokes an existing consent and issues a new `sid` for reconnect.

Use WorkOS-only sign-in and fresh Ding state. The owner confirmed that the
existing deployment is disposable development data and requested a reset; no
legacy account migration is needed. Retain the WorkOS identity so its next login
creates a fresh personal workspace. Email matching is never Ding authorization.

**Acceptance:** fresh users connect without Console setup or technical input;
first login works without notification credentials; canceled consent creates no
usable connection. Keep the first release to one personal workspace. Organization
and multiple-workspace flows need explicit mapping and are not release claims.

## 3 Make connection and reconnection work naturally in ChatGPT

Declare the OAuth scopes on each hosted tool and return the appropriate
`_meta["mcp/www_authenticate"]` signal when authentication or additional OAuth
permission is needed. Keep policy denial distinct from expired authentication.
Test the resulting linking prompt in ChatGPT, including the embedded UI.
[OpenAI tool-level authentication](https://developers.openai.com/plugins/build/auth#triggering-authentication-ui).

Provider-backed cloud grants now renew their private thirty-day window on verified
requests while retaining a durable consent hash and revocation tombstone. Access tokens remain short-lived; token refresh must never
restore a disconnected grant or expand scopes. Retain immediate membership and
permission checks on every call. Scope increases require another explicit
approval. Existing local and temporary grant expiry rules need not change.

One personal workspace connects without a picker. Unsupported organization or
multiple-workspace contexts currently fail closed; extend only with explicit mapping. Show a clear
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
OIDC/MCP authority, enable hosted sign-in and deploy a pinned revision using
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

Complete the publisher identity verification already requested by OpenAI's
portal, then verify the endpoint domain, upload the cloud ZIP, complete
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
| Hosted account/consent qualification | WorkOS clients and real callback | Fresh users connect without Console setup |
| Provider refresh and reconnect qualification | Implemented consent tombstones | Live refresh succeeds; new consent reconnects |
| Hosted auth metadata and errors | Selected scopes | Real ChatGPT linking/relinking prompts |
| Cloud package and first-watch guidance | Fixed endpoint and client mode | Valid ZIP and clean test installation |
| Railway pilot qualification | Above changes reviewed and deployed | Actual ChatGPT journey with laptop-off delivery |
| Marketplace submission and launch | Pilot, publisher and public admission ready | Approved listing and clean marketplace install |

Keep these as small, reviewable changes. The immediate next step is live WorkOS/ChatGPT qualification and deployment. Further
local/cloud transfer work, native installers and ChatGPT wakeups are not on this
critical path.
