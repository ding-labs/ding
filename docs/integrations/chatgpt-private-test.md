# Private ChatGPT qualification

This personal development plugin exercises the production customer path before
public publication. It uses the same HTTPS MCP endpoint and WorkOS sign-in as
the eventual marketplace plugin, with a separate OAuth client. Public identity
review does not block this private connection.

## Registered test connection

Recorded 2026-10-12. All identifiers below are public, not credentials.

| Setting | Value |
| --- | --- |
| ChatGPT name | Ding — Test |
| ChatGPT app | `asdk_app_6acc78506730819190b95f1efb26794e` |
| WorkOS application | `app_01M4QB6TNWYE9SMBASFND4XJDZ` |
| OAuth client | `client_01M4QB6TNVKZ1B53KJWZXRYXQA` |
| Exact callback | `https://chatgpt.com/connector/oauth/Z4dzR1htvI-k` |
| MCP URL / resource | `https://cloud.ding.ing/mcp` |
| Client registration / authentication | User-defined public client / `none`, S256 PKCE |
| Default scopes | `ding:inspect ding:preview ding:manage ding:retry` |
| Base scopes | `openid offline_access` |

ChatGPT discovered the endpoints from Ding's protected-resource metadata and
WorkOS discovery. OIDC is enabled. WorkOS displayed identity, email, ongoing
access, and the four watch permissions. The owner's existing hosted session
completed consent and ChatGPT showed the account connected in development mode.
The live checks below extend that initial sign-in and token-exchange evidence.

The production marketplace client remains separate and still needs the callback
from its own portal registration. Never copy this private callback into it.

## Deployment

Keep the existing `DING_AUTHKIT_JSON` values and add:

```json
"additionalChatgptClientIds": ["client_01M4QB6TNVKZ1B53KJWZXRYXQA"]
```

There may be at most ten distinct MCP clients in total. The browser client,
duplicates, blank IDs and whitespace are rejected. Removing a client from the
allowlist disables even previously bound credentials for that client.

Set `DING_AUTHKIT_API_KEY` as a private Railway variable to enable hosted consent
revocation. Never put it in source control, plugin manifests, ChatGPT, test
transcripts or watch credentials. Sign-in and token verification do not use the
key. Disconnect uses it to list the verified user's authorized applications,
match the exact OAuth client ID, and revoke that application's authorization.
The API origin is fixed to WorkOS and redirects are refused.

Deploy the tested commit and variables together. No database reset or schema
change is required. Check readiness, protected-resource discovery, and the
unauthenticated 401 challenge before testing the connected plugin.

## Live qualification evidence

On 2026-10-12, Railway successfully deployed commit
`254ad1c65d2f5162f538a46723441dbbad674aea` as deployment
`8ec6a26f-30f7-426a-b216-aabaaecfaad6`, with the private client allowlisted and
the WorkOS revocation key configured as a server-only variable. Readiness returned
200 and unauthenticated MCP returned 401. All GitHub checks for that code passed.

The owner's private ChatGPT conversation verified:

- Authenticated capabilities, workspace discovery, cloud usage and watch listing.
  The embedded Ding view showed the personal workspace without manual binding.
- Invalid previews rejected without saving a watch; a valid preview displayed
  the exact proposed watch before approval.
- Applying the approved preview created only `chatgpt-private-test`. The first
  cloud check ran; no incident or notification delivery was recorded.
- Pausing through ChatGPT produced generation 2. Console independently showed
  the same watch paused, with its definition intact.
- Console disconnect returned 200 and removed the model binding. ChatGPT's next
  request required reconnect. WorkOS displayed fresh consent for the same client.
- Cancelling that consent returned to ChatGPT and did not recreate a Ding binding.
- Completing a subsequent consent restored authenticated workspace discovery and
  watch inspection. The original watch remained paused at generation 2 with
  revision `7739ce6c212268d700ff721348491beae8404f62108629a211851e1991cae053`.
  No second Console setup or new workspace was needed.

The watch remains paused for future qualification. It has no credentials or
notification destinations. Use this known-good cloud fixture for repeatability:

```yaml
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata:
  id: chatgpt-private-test
  name: ChatGPT private connection test
spec:
  source:
    type: http
    url: https://cloud.ding.ing/readyz
    every: 5m
    timeout: 10s
  condition:
    field: http.status
    operator: ne
    value: 200
  policy:
    trigger: transition
    consecutive: 1
    recoverAfter: 1
    onUnknown: hold-incident
  destinations: []
  limits:
    maxBytes: 65536
    maxEntities: 10
    maxSamples: 100
    maxOutputs: 10
```

One authoring gap surfaced: without a manifest example/schema in its tool
context, ChatGPT repeatedly guessed cadence fields and omitted required cloud
limits. The explicit fixture above succeeded. Before public qualification, make
the watch schema and a valid cloud example discoverable through MCP and repeat
an unaided natural-language creation test. The development widget also displayed
"CSP off"; qualify the declared widget CSP with enforcement enabled before
publication.

Access-token refresh after expiry, controlled delivery retry, and a fresh GitHub
provider round trip are not yet demonstrated by this session. Do not count
reconnect as refresh or infer these outcomes from unit tests.

## Repeatable live checks

Start a new ChatGPT conversation with **Ding — Test** selected. Refresh tools in
the plugin settings after changing tool definitions. Record the deployed commit,
date, account/provider (without credentials), operation receipts and outcome.
Use `chatgpt-private-test` as the watch ID and a controlled HTTPS fixture; never
point destructive or retry tests at unrelated watches or real notifications.

| Check | Prompt or action | Required evidence |
| --- | --- | --- |
| Discovery | “Use Ding — Test to show my workspace and Ding capabilities. Make no changes.” | Cloud execution, correct workspace, successful authenticated tools |
| Read | “List my Ding watches and recent runs.” | Correct user's data; no manual Console binding |
| Preview | “Preview a watch named chatgpt-private-test against our test HTTPS fixture. Do not save it yet.” | Preview is read-only and reports a concrete proposed configuration |
| Create | “Save that reviewed test watch.” | Reviewed plan applied once; receipt visible |
| Change | “Preview changing the test watch's interval, then wait for approval.” | No mutation before approval; apply after explicit approval |
| Lifecycle | “Pause chatgpt-private-test.” | Only test watch pauses; same state in Console |
| Delivery | Trigger the fixture's controlled failing destination, then request one retry | Correct delivery receipt; no duplicate notification |
| Refresh | After access-token expiry, read the same watch in a new turn | No browser prompt; same workspace and grant |
| Disconnect | Disconnect this model in Ding Console | Local access stops immediately; provider consent is removed |
| Reconnect | Reconnect the account from ChatGPT plugin settings | New WorkOS consent, usable tools, same watches/workspace |
| Cancellation | Cancel a new OAuth authorization | No access or new account grant created |
| Social providers | Repeat a fresh sign-in with Google, then GitHub | Real provider round trips, correct account, no unexpected duplicate workspace |

If disconnect cleanup fails, Console shows **Access disabled; disconnect cleanup
pending** and **Retry disconnect**. The disabled binding survives restart and
all tokens for it stay blocked until cleanup finishes. Retry is idempotent and
does not change other clients' grants. Old consent tokens remain blocked after
successful reconnection. Automated tests cover this failure path and isolation;
live WorkOS behavior still requires the checks above.

Google social sign-in was confirmed working by the owner, including initial
email verification. The private connection above reused a hosted session; it
does not count as a new Google or GitHub round trip. GitHub remains unqualified.

## Keep or remove the private plugin

Keep this private client for future version qualification if useful. It is not
packaged into the public plugin. It currently reaches production, so use only
dedicated test watches; a private client alone does not isolate runtime changes.
Use a separate staging deployment when future changes need isolated execution.

To retire it: disconnect in Ding Console to revoke consent, remove the private
ChatGPT plugin, remove its additional client ID from the deployment profile,
then retire its WorkOS application. Keep the public client and browser client.
Application deletion is permanent and should be explicitly approved at that
time. Removing the plugin requires no Ding code removal or account deletion.

Public release still follows the [publication runbook](chatgpt-cloud-publication.md):
approved publisher identity, public callback, complete plugin package, public
support/privacy/terms pages, reviewer access, fixtures and successful review.

Sources: [ChatGPT connection testing](https://developers.openai.com/plugins/deploy/connect-chatgpt),
[OpenAI OAuth](https://developers.openai.com/plugins/build/auth),
[WorkOS authorized applications](https://workos.com/docs/reference/authkit/user#delete-an-authorized-application).
