# Complete the ChatGPT first-watch journey

Proposed November 5, 2026. Implements outcome 2 of the
[adoption plan](developer-adoption-plan.md). Increments 1–2 now have implementation
and automated qualification as recorded below. The remaining behavior and real-host
qualification are still planned.

## Outcome and boundaries

An engineer connects Ding in ChatGPT and asks: “Watch my service and tell me when
it starts failing.” They select an owned public endpoint and alert destination,
review the exact rule, confirm a labeled notification, activate once, see the
first observation, and later ask ChatGPT to explain an alert from retained
evidence. They never edit YAML or paste credentials into the conversation.

Use hosted ChatGPT on the web as the first qualification target. Record the
actual product surface, account/workspace eligibility, model, plugin build, and
server commit; do not infer support for every ChatGPT or Claude surface. Preserve
local use and the existing private connection. Billing, transfers, public
marketplace submission, and automatic chat wakeups are outside this milestone.

The current custom MCP connection is useful for testing tools, but does not prove
that the packaged skills are installed or invoked. Qualify both the connected
server and the complete plugin. Current OpenAI guidance treats these as distinct
tests and calls for refreshing changed metadata before a new conversation.
See [connection and plugin testing](https://developers.openai.com/plugins/deploy/connect-chatgpt).

## Verified starting point and actual gaps

- PR #73 deployed compiled-rule descriptions, saved-destination reuse, durable
  setup-test receipts, and first-input confirmation. PR #74 records production
  and fresh OAuth evidence. See [qualification](first-watch-qualification.md).
- MCP already supports capabilities, workspace selection, preview/apply,
  operation reconciliation, watch/event/delivery inspection, and lifecycle actions.
  Reuse those authorization and persistence paths.
- The cloud package already has setup/watch skills and a manifest reference.
  They do not yet walk through the exact tested starter journey. Server-only
  connections may not receive those files. An earlier real-host run guessed
  invalid manifest fields; the current generic preview still requires a manifest.
- Destination testing currently requires protected browser/admin authority. The
  four MCP scopes do not grant destination administration. There is no complete
  model-readable return path for browser setup and receipt confirmation.
- Reconnect succeeded, but the email-label discrepancy required manual account-ID
  inspection. No designated authenticated profile tool currently supplies that
  identity to the host.
- Owner-session success does not establish new-user activation, packaged-skill
  discovery, expired-token refresh, or real-host failure/fallback behavior.

## 1. Make account continuity understandable

Add a hosted, authenticated, read-only `ding_get_profile` tool with empty inputs.
Return the immutable Ding user ID as the opaque profile ID and available display
name; omit unavailable email rather than inferring it from another account.
Workspace selection remains a separate operation. Resolve the profile from the
validated request identity before any ambiguous workspace selection. Never use
email, grant ID, session ID, or workspace name as the profile identity.

Declare `_meta["openai/profile"]: true` and the documented profile schema. Return
the profile object directly in structured content plus its JSON text equivalent,
not inside Ding's ordinary view envelope. Keep the normal tools unchanged. Use
existing permissions; this is not account linking or a consent expansion. The
metadata does not guarantee host eligibility or label behavior; verify those in
the actual client. [OpenAI profile contract](https://developers.openai.com/plugins/build/auth#support-multiple-accounts).

**Acceptance:** reconnect and display-name/email changes preserve the profile ID;
different authenticated users get different IDs; revoked/invalid credentials fail;
the tool cannot select another user. Host settings and conversation discovery
identify the intended account without asking the user to remember email aliases.

**Commits:** profile resolver and isolation tests; MCP schema/metadata adapter and
interoperability tests. Touch `internal/cloud`, `internal/mcpserver`, and
`internal/mcpcontract`; avoid unrelated identity migrations.

## 2. Give the assistant a reliable starter preview

Add a preview-only `ding_preview_service_watch` tool for the supported HTTP starter.
Inputs: watch ID, endpoint URL, existing destination reference, and the existing
optional workspace selector. Support an explicit expected HTTP status as a typed
option; otherwise use the three-5xx/two-recovery recipe. Reject ambiguous options.
Reuse the existing template/compiler and hosted policy, extracting shared code if
needed rather than importing the cloud server into the MCP adapter.

Route the generated manifest through the existing scoped preview service. Return
its ordinary expiring handle, canonical rule descriptions, actual cadence and
timeout, execution target, and reviewed destination revision. Apply continues
through `ding_apply_changes` with its existing stable operation key. Do not add
a second apply engine or let a Console review token substitute for an MCP handle.
Preview must neither fetch the endpoint nor send a notification. The starter
requires a destination; advanced authoring remains available through generic
preview and the packaged reference.

**Acceptance:** a natural-language request reaches a valid preview without model-
authored YAML; local/private URLs are rejected by cloud policy; stale destinations,
duplicate IDs, unsupported intervals, and unavailable references are actionable.
Generated local/cloud rules and UI descriptions stay consistent. An explicit
expected-status request is honored rather than silently replaced with 5xx-only.

**Commits:** shared recipe extraction and behavior tests; typed preview contract
and routing; schema/SDK compatibility fixtures. Test through the actual hosted
HTTP handler as well as SQLite/PostgreSQL-backed preview/apply contracts.

## 3. Complete protected destination setup and return

Keep credential entry, destination administration, and setup-test sends in the
authenticated Console. Add a focused destination-only setup view using existing
saved-destination and new-credential components; it must not create a second watch.
The assistant links there when a destination is absent or unconfirmed, then
continues after the user returns. Do not promise automatic reopening of the chat.

Add a narrow, read-only setup-status contract for the authenticated workspace and
selected destination. Persist receipt confirmation from the browser and bind it
to the authenticated user/workspace, destination revision, credential version or
digest, and test operation. Keep secret values and digests out of model results.
Expose separate states for missing setup, pending/uncertain test, accepted test,
and user-confirmed receipt. A URL or opaque setup ID is not an authorization token.
Require current access on every lookup; never accept a model argument asserting
that a notification was seen. Define a bounded confirmation validity window and
invalidate readiness after destination/credential changes without deleting the
durable no-resend receipt.

For this first-watch journey, return to ChatGPT, refresh destination/readiness,
obtain a fresh preview, explain it, and apply under the user's existing
authorization. Generic advanced MCP apply remains a separate capability; do not
describe this onboarding guard as a universal server-side notification requirement.
If the browser is signed into another account, show the mismatch and a normal
sign-in path rather than silently switching targets or linking by email.

**Acceptance:** both an existing destination and a first-time destination complete
the round trip; no secret enters chat; a lost test response reconciles without
another send; a canceled setup creates no watch; wrong-account, stale, expired,
and cross-workspace status requests do not unlock onboarding.

**Commits:** scoped readiness persistence/read API and backend tests; protected
browser setup/confirmation with real-daemon browser tests; MCP read-only status
contract. Do not grant destination-administration authority to existing MCP scopes.

## 4. Teach and package the complete conversation

Update the shared and cloud-specific setup/watch skills and their references.
Use one explicit sequence: discover account/workspace and permissions; collect
endpoint and notification intent; reuse/setup/test destination; preview; explain
the exact rule; apply within authorization; reconcile an interrupted response;
read the watch until first input or a bounded timeout; offer evidence and a second
watch. Report waiting honestly instead of polling indefinitely. Never promise
immediate detection at a five-minute cadence or future replies in an unsubscribed
chat. Explain alerts from event-time observations and delivery attempts, separating
facts from hypotheses and current conditions from retained evidence.

Update tool descriptions/server instructions so the server-only connection also
has an actionable route. Update package examples and review prompts to actual
tool names. Test the generated cloud package, not only source skill files; do not
assume shared-skill edits propagate into the cloud-specific copies. Preserve
platform-specific differences. [Plugin packaging](https://developers.openai.com/plugins/build/plugins).

**Acceptance:** direct, indirect, follow-up, and unsupported prompts select the
right workflow. “Preview only” causes no apply or test send. Prior authorization
does not cause repeated approval questions. An uncertain apply keeps the same
operation key and target; absent or stale reviews never cause an invented apply.

**Commits:** shared/cloud skill journey and executable evaluation cases; package
metadata/references and generated-artifact validation. Add tests for meaningful
tool and side-effect behavior rather than snapshots of instruction wording.

## 5. Keep embedded UI and text equally usable

Review the existing MCP App against the starter result types and add only missing
states. Both forms must report account/workspace, execution location, rule,
cadence, first-input timestamp, source health, incident state, and delivery state.
Provide evidence links with event IDs. A failed or unavailable embedded view must
leave a complete text path; it must not cause another apply or test notification.
Verify declared CSP with enforcement enabled, keyboard access, and state restored
after reopening the component. Inspect-only grants must not offer usable write
actions, and server authorization remains authoritative.

**Acceptance:** identical synthetic results produce consistent facts in text and
UI; missing history/replay and failed deliveries stay visible; untrusted source
text cannot direct tool calls. Exercise both the real embedded host and text-only
fallback, retaining screenshots and sanitized transcripts.

**Commits:** missing UI states with focused tests; text/result parity and host
evaluation fixtures. Run `make test-mcp` and the relevant Console/browser suites.

## 6. Qualify a fresh participant and recoverable failures

First verify the intended participant can install/use the private packaged plugin
on the chosen host. Record any workspace-policy restriction before inviting them;
do not silently substitute a developer's account or declare a server-only test a
packaged-plugin pass. Participant outreach is a separately authorized action.

Use a participant-owned test account, an owned public endpoint, and an explicitly
approved notification destination. Do not clear the owner's production data or
create a fake participant. The engineer should not need terminal commands,
bearer tokens, client IDs, YAML, or researcher-supplied tool arguments. Record
every intervention and distinguish assisted runs from independent completion.

| Scenario | Required evidence |
| --- | --- |
| Fresh install and sign-in | Actual new-user connection; correct profile/workspace; valid tools; no existing provider session counted as fresh GitHub sign-in |
| Existing and missing destination | Successful protected setup/return; confirmed notification; no secrets in transcript |
| Preview, activate, first input | Reviewed rule; one apply effect; committed watch; first-observation time recorded separately |
| Firing, recovery, explanation | Controlled public fixture; real notification read back; event IDs, retained observations/replay, and delivery outcome correctly explained |
| Private URL, missing credential, read-only grant | Useful rejection; no unauthorized source execution or writes |
| Changed destination, expired preview, interrupted apply | Fresh review where needed; same-key reconciliation; one committed effect |
| Token expiry | Authorized calls after actual access-token expiry without interactive reconnect; same account/workspace; sanitized provider/server evidence distinguishes refresh from cached-token use |
| Disconnect, cancellation, reconnect | Access ends, watches continue; cancellation creates no usable grant; reconnect preserves identity/data; old consent remains denied |
| UI unavailable and hostile evidence | Usable text workflow; no duplicate actions; source instructions remain data |
| Close ChatGPT and laptop-off check | Hosted samples/authorized alerts continue, verified from another device or observer; no claim based only on closing a browser tab |

Use an isolated controlled HTTPS fixture for failure scenarios. Qualify it in a
test environment before any narrowly scoped production smoke test. Preserve real
hosted polling policy; record deliberate test-recipe differences. Use authorized
test-only writes and bounded notifications, then pause qualification watches and
retain evidence. Refresh testing must not shorten production token lifetimes;
use natural expiry or separately configured test-provider behavior. Long waits
may proceed separately from shipping unrelated verified fixes.

## Completion, sequence, and ownership

Engineering owns increments 1–5, automated checks, packaging, and deployment
evidence. The account operator owns test-account/host access and approved delivery
destinations. A researcher observes a fresh engineer for increment 6. The owner
session is a prerequisite smoke test, not the independent acceptance result.

Start with increment 1 because the observed account confusion is concrete and
bounded. Follow with 2, then 3, then 4–5, then the fresh-participant run. Establish
host/package eligibility early while building. Estimate 8–12 engineering days,
then adjust after the protected-setup contract; scheduling participants and token
expiry adds elapsed time. Do not call the milestone complete until a fresh
engineer completes the journey without YAML, secret sharing, or implementation
coaching and the required host scenarios have recorded outcomes.

Record connect/install time, activation time, first-input time, notification
receipt, interventions, and whether the engineer can create a second distinct
watch. Aim for activation within five minutes after connection/setup; report
actual timings rather than excluding setup friction from the user experience.
This milestone's usability evidence does not replace the ten-engineer pilot's
day-7/day-30 repeat-use or payment measurements.

Commit each behavior with directly related tests before the next independent
change. Preserve small commits in PRs. Run focused checks per increment and the
full relevant Go race/vet, PostgreSQL, Console, SDK, and MCP App suites at release.
Add no unrelated checkout changes. Both 24-hour soaks remain optional, non-blocking
background validation with separately reported actionable failures.


## Implementation record: account profile and typed preview

November 5, 2026. Implemented in an isolated checkout; unrelated working-tree
changes were preserved. These results cover repository code, not a new production
deployment or a real ChatGPT run with refreshed metadata.

- `c12d532`: hosted `ding_get_profile` resolves the immutable Ding account before
  workspace selection, returns the direct profile contract, and checks current
  consent. Signed-token HTTP tests cover label changes/reconsent, linked identity,
  same-email distinct accounts, multiple workspaces, revoked/invalid credentials,
  and rejection of user/workspace selector arguments.
- `dee869e`: extracted the common Console/MCP HTTP service recipe. Local and Cloud
  compiled definitions retain their existing defaults; explicit expected HTTP
  status is supported. The Cloud byte limit uses the network policy constant.
- `909a5dd`: typed preview API uses the normal grant, policy, review, and persisted
  handle paths. SQLite and real PostgreSQL tests cover changed destinations,
  cross-grant rejection, restart, duplicate apply receipts, existing watch IDs,
  and a watch created between preview and apply. No alternate apply engine.
- `04db452`: added `ding_preview_service_watch`, strict input schema, scope metadata,
  routing, and `ding.mcp/v1.3`. Hosted HTTP tests cover default/explicit-status
  rules, descriptions, target, private URLs, missing destinations, insufficient
  scopes, cross-workspace rejection, no preview I/O, and ordinary apply replay.
- `49cda1e`: the combined adapter also exposes the starter with explicit connection selection;
  its SDK test rejects an ambiguous target and missing destination without fallback.

Qualification passed:

| Check | Result |
| --- | --- |
| Go race tests | Cloud, MCP server/contract/client/bridge, onboarding, control, access, and watch runtime checks passed |
| Real PostgreSQL | `TestPostgresServicePreviewPreservesReviewAndReceipts` passed against the isolated PG17 fixture |
| Static checks | Relevant Go vet, gofmt, and `git diff --check` passed |
| `make test-mcp` | Passed: Go race/package checks, 7 UI unit tests, independent TypeScript SDK against the real daemon (19 local tools), and 2 MCP App browser tests |
| Hosted protocol contract | 22 hosted tools; profile shape/metadata and OAuth scopes verified through the real HTTP handler with synthetic signed tokens |

Still pending for increments 1–2: production deployment, refreshed host metadata,
actual ChatGPT profile/account-label behavior, and natural-language tool selection
in the completed onboarding workflow. No claim of new-user or packaged-plugin
qualification is made. The next implementation is increment 3: protected
destination setup, receipt confirmation, and model-readable readiness bound to
current account/workspace/destination state. Do not use preview success as proof
that a notification destination was tested or a notification was seen.
