# ChatGPT embedded-view loading investigation

November 5, 2026 Pacific (November 6 UTC). Follow-up to the
[packaged journey qualification](chatgpt-journey-qualification.md).

## Outcome

The resource-cache contract is fixed in commit `58d9a0c`,
[PR #80](https://github.com/ding-labs/ding/pull/80). The intermittent initial
inline-loading failure is **not fully resolved**. A fresh watch view loaded
without retry after deploying the fix, but the next events view failed before
the Ding component was inserted. The host's **Open app in tab** control opened
that same result successfully in its side viewer. Earlier **Try again** actions
also recovered views without replaying watch mutations.

## Cache correction

Ding previously changed its bundled HTML/JavaScript while always advertising
`ui://ding/workspace.html`. Hosts treat the resource URI as a cache key; server
version changes alone do not invalidate it. See the
[official component-resource guidance](https://developers.openai.com/plugins/build/chatgpt-ui).

Native, hosted, and combined servers now advertise
`ui://ding/workspace-<SHA-256-of-bundle>.html`. Tool descriptors, resource listings,
and resource contents agree on that address. Identical bundles keep the same
address; changed bundles get a new one. The original unversioned URI remains
readable for older conversations. No tool scope, watch state, or credentials
changed. SDK/browser tests now discover the advertised resource rather than
reading a hard-coded URI.

## Live observations

- Before the fix, two new read-only requests in the existing owner conversation
  failed to display their inline watch view. A short one-sentence assistant
  response did not prevent the failure. Host retry loaded the same result.
- Candidate `58d9a0cc87ebd2e9fbb4a10d907f9b740013ff42` deployed successfully as
  Railway deployment `fe83d377-f2f3-44c7-900f-a38963fbc203`. Health, readiness,
  and protected-resource discovery returned 200. The existing ChatGPT connection
  was refreshed and a new conversation started; no reconnect or consent change.
- The fresh conversation's first read-only watch inspection loaded its connected
  component without retry. It showed the qualification watch paused at generation 2.
- The next read-only events request completed at approximately `04:35:19Z`, but
  its inline component remained at Opening and then showed Couldn't display.
  The visible outer sandbox iframe was 480 pixels high. Its body was observed
  empty, with no inner component iframe, while it was stalled.
- A separate hidden preload iframe can also exist. Frame observations must be
  matched to the visible element's bounds and exact source; the last iframe in
  DOM order is not reliable evidence of the visible result's state.
- The inspected deployment's MCP HTTP requests during the fresh conversation
  returned 200 in 39–113 ms, with no proxy errors. HTTP status alone does not
  establish JSON-RPC success for every request; no method-level network trace
  was captured. Browser logs exposed extension warnings but no actionable Ding
  error. Do not attribute the failure to those extensions without isolation.
- Selecting **Open app in tab** on the failed events result rendered the two
  retained creation/pause events in ChatGPT's side viewer and connected to the
  correct workspace. It did not repeat a watch action or notification test.
- The existing global CSP setting was inspected and left unchanged. This is not
  a production-CSP host qualification. Offline browser tests enforce Ding's
  restrictive no-network component CSP separately.

This evidence places the observed stall before Ding's React/bridge startup.
It does not identify the precise host/bootstrap cause. Cache invalidation was a
real contract defect, but the post-fix failure demonstrates it was not sufficient.
Changing Ding's in-component retry logic cannot recover a document the host has
not inserted. Preserve this distinction in release notes and support reports.

## Verification and preservation

- Focused Go contract/server/bridge tests passed, including content-address
  stability, changed-bundle invalidation, readable legacy resources, and reading
  the address advertised by each UI tool through the real SDK.
- MCP race checks, seven UI unit tests, independent TypeScript SDK interoperability,
  and both embedded-browser tests passed. A broader run hit an unrelated
  log-ingestion timing failure; three isolated repetitions passed afterward.
- All eight existing watches retained their revisions, generations, and statuses
  after deployment and live reads. The qualification watch stayed paused at
  generation 2; the saved Slack destination revision was unchanged.
- PR #79 merged as `b8d0439` only after 25 checks passed and one deployment check
  was appropriately skipped. Its failed macOS idle-runtime timing assertion
  passed on rerun. All three commits were preserved.
- PR #80 merged as `81e89a8` after all 47 checks passed. The service source was
  restored to follow `main` without a commit pin after candidate qualification.
- The original dirty checkout was untouched. Both 24-hour soaks remain optional,
  non-blocking background validation.

Private screenshots and the visible transcript were retained locally, including
the post-fix failure and successful side-view recovery. They are not committed
because they include the owner's chat UI. The private conversation/account IDs
are deliberately omitted here.

## Recovery and next diagnostic

Use the host's **Try again** or **Open app in tab** controls for a failed view,
or continue with the text result and Ding Console. Do not reapply a preview,
resume a watch, or resend a notification merely to reload a component. A tool's
successful return does not prove that its embedded view rendered.

The remaining investigation needs a host-side trace for the visible sandbox
initialization: correlate frame creation, resource retrieval, inner-frame insertion,
and initialization messages for a failed first open and successful manual open.
Retain both timestamps and the distinction between visible and preload frames.
The reproduction is read-only: inspect the paused qualification watch, then list
its recent events in a fresh ChatGPT conversation. No credentials, account reset,
or notification sends are needed. No external support message has been sent.
