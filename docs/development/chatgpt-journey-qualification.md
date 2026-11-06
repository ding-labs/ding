# Packaged ChatGPT journey qualification

Run November 5 Pacific / November 6 UTC, 2026. This is an assisted owner-session
qualification of the private **Ding — Test** plugin in ChatGPT web on Chrome.
It is not independent new-user acceptance or a public marketplace submission.
The host displayed Medium thinking effort; an exact model identifier was not
captured. See the [journey plan](chatgpt-first-watch-plan.md) for full acceptance.

## Build and deployment

- PRs [#76](https://github.com/ding-labs/ding/pull/76) and
  [#77](https://github.com/ding-labs/ding/pull/77) supply stable profile identity,
  typed starter preview, protected destination setup, and durable readiness.
- [#78](https://github.com/ding-labs/ding/pull/78) packages the complete shared/cloud
  setup and watch instructions, review prompts, archive tool/link validation,
  and an explicitly private build mode. Public builds still require policy URLs.
- Production deployed merge `f2072bc46c4ee87c150979b74b491761115d5373` as Railway
  deployment `9c7666ce-39ab-41aa-9e57-a31cbe2051b5`. Readiness succeeded at
  `2026-11-06T03:47:27Z`. Health and readiness returned 200; unauthenticated MCP
  returned 401; `/.well-known/oauth-protected-resource/mcp` returned 200.
  Live capabilities reported `ding.mcp/v1.4`.
- Existing account, workspace, consent, destination, database, and volume were
  retained. No account linking, permission expansion, schema migration, or data
  reset was performed. The service source now explicitly follows `main`.
- [#79](https://github.com/ding-labs/ding/pull/79) adds the compatibility packager
  required by the existing ChatGPT app. A standalone archive was rejected for
  its different plugin name. Exporting the installed plugin and preserving its
  host-assigned name and `.app.json` binding allowed an in-place version 0.2.0
  update with both skills enabled and the existing OAuth connection retained.
  The compatibility archive deliberately omits a second MCP connection.

Artifact SHA-256 digests:

| Artifact | SHA-256 |
| --- | --- |
| Portable private cloud 0.2.0 | `0e0f53a211f50eb3f4bed5fa432ea9fdba04bca45b812f2f9b47252ee15f4ec9` |
| Installed existing-app update | `e89fc598dbd882fa800a20305c5e008cc0c5dcaec4afbb15941752f7b680cb7a` |

The portable archive was built from `f6793dc`; the compatibility adapter was
committed in `7e8e582`. Neither artifact is approved for public distribution.
Published service privacy/terms URLs remain an operator prerequisite.

## Observed live journey

After refreshing the plugin's tools, a new ChatGPT conversation requested a
preview of `qualification-chatgpt-journey-20261106`, checking
`https://cloud.ding.ing/healthz` for HTTP 200 using saved Slack delivery.

| Step | Evidence and outcome |
| --- | --- |
| Preview only | Valid review without user-authored YAML: five-minute cadence, five-second timeout, three non-200 observations to fire, two healthy observations to recover, separate source-health handling. Independent watch listing found no saved watch; no test was sent. |
| Saved destination | Reused `slack-ding-notifications`; no credentials requested in chat. Readiness was untested and the assistant supplied a protected setup link. |
| Identity | ChatGPT reported the authenticated immutable account ID and workspace, matching the setup page without relying on email aliases. A deliberately mismatched account link was blocked by the live page. |
| Labeled test | Operator-authorized test accepted at `03:50:33Z`. The user explicitly confirmed actual Slack receipt in this Codex chat; browser receipt confirmation was then recorded at `03:51:38Z`. No Slack connector readback is claimed. |
| Readiness return | ChatGPT refreshed and reported confirmed receipt, valid until `2026-11-07T03:50:33Z`. It did not invent confirmation from provider acceptance. |
| Activation | One new watch appeared, generation 1. First input was recorded at `03:51:48.40229622Z`, with zero matches, no open incident, and no source-unhealthy condition. The inspection did not expose the exact raw HTTP status, so HTTP 200 itself was not independently verified from that result. |
| Pause and inspect | At the operator's request, ChatGPT paused only this watch at `03:53:24.11242451Z`, generation 2, preserving creation/pause events and history. No notification deliveries were present. A complete text result and Console link remained usable. |
| Unsupported endpoint | A request for `http://127.0.0.1:8080/admin` was rejected at preview with the standard-port policy error. No watch was created. This result alone does not independently test loopback rejection on a standard port. |

Independent MCP reads at `04:01:21Z` found exactly eight watches: the seven
baseline watches retained their revisions, generations, and lifecycle states;
the sole new qualification watch remained paused. The saved Slack destination's
revision was unchanged. No second test or unintended watch was created.

## Host behavior and qualification limits

- The installed plugin visibly contained both versioned skills. The conversation
  produced the supported starter policy, but the visible host summary did not
  independently reveal the exact preview tool name or prove skill invocation.
- The assistant refused to fabricate receipt when asked to treat acceptance as
  confirmation. That turn overlapped the user's genuine browser confirmation
  and also activated the watch under explicit authorization. Therefore it does
  **not** cleanly qualify refusal to activate while unconfirmed. The response
  treated activation as separately authorized. Readiness guidance is not a
  universal server-side gate on advanced MCP apply.
- Embedded results initially failed to display. After revisiting the conversation,
  the actual Ding List Events widget rendered; navigation to Watches and the
  paused watch's details worked through the host bridge. This is intermittent
  rendering evidence, not a root-cause diagnosis or a clean UI pass. The host's
  existing development CSP setting was not changed; enforcement-on qualification
  remains pending. Text fallback completed the journey without repeated writes.
- The final inspect link opened the Console, not a watch-specific deep link.
- This run reused the owner's current account and provider session. Earlier
  [fresh reconnect evidence](first-watch-qualification.md#fresh-oauth-reconnect-passed)
  remains valid, but this run does not establish a fresh GitHub sign-in,
  expired-token refresh, independent participant installation, a first-time
  destination, controlled firing/recovery, laptop-off behavior, or all recovery
  and hostile-evidence scenarios in the plan.

Private screenshots and the visible conversation transcript were retained locally;
account identifiers and the private conversation URL are omitted from this report.
The original dirty checkout was untouched; work used an isolated checkout and
small, unsquashed commits.

## Automated verification

- Focused Go package/contract tests passed for `internal/mcpcontract`,
  `internal/pluginpackage`, and `cmd/package-integrations`.
- `make test-mcp` passed: Go checks, independent TypeScript SDK interoperability
  against the real daemon with 20 local tools, seven UI unit tests, and two
  embedded-app browser tests.
- The Python compatibility archive test passed, covering identity preservation,
  shipped skills, omitted duplicate MCP configuration, mismatched app rejection,
  and refusal to overwrite an output archive. It also passed in #79 CI.
- #77's complete CI was green. At report preparation, #78 and #79 had no failed
  checks, but some platform/build/Console jobs remained running or queued; this
  is not a claim of completed green CI for those PRs. Their live check status is
  authoritative.
- Both 24-hour soaks remain optional, non-blocking background validation and are
  not release gates.
