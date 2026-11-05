# From first watch to paid hosted use

Proposed November 5, 2026. Implementation plan for five outcomes: a useful service
watch, an assistant-driven setup, a ten-engineer pilot, a paid hosted offer, and
an event-triggered investigation. This document does not claim those outcomes
have been achieved or authorize outreach, charges, or deployment.

## Product decision

Lead with **“Stop checking. Ding will tell you.”** Start with engineers who have
just shipped a service and want to stop checking its health endpoint manually.
The first complete journey is:

1. Supply an owned endpoint and choose where alerts should arrive.
2. Review exactly what Ding checks, its cadence, and where it runs.
3. Confirm receipt of a labeled destination test and activate the watch.
4. See the first real observation and know monitoring is running.
5. Receive a firing/recovery event and inspect the evidence.
6. Create another useful watch or choose hosted execution.

Use hosted ChatGPT as the initial assistant target, with a manual custom
connection for the pilot. It provides a path to the later event demonstration.
Retain existing local and Claude support; marketplace publication and broader
client qualification remain separate work. If hosted OAuth access is delayed,
start Console participants while qualifying the assistant route.

The main adoption target is **five of ten enrolled engineers independently
creating a second useful watch within 30 days**. This is a directional experiment,
not proof of market size or a statistically conclusive retention benchmark.

## Existing foundation and boundaries

Reuse the runtime, compiler, delivery outbox, evidence, Console onboarding,
reviewed MCP mutations, and plugin packaging. Relevant starting points:

| Area | Existing implementation or plan |
| --- | --- |
| Local starter watch | `internal/onboarding/watch.go`, `internal/control/onboarding.go`, `web/console/src/pages/FirstWatch.tsx` |
| Hosted starter watch | `internal/cloud/api.go`, `web/console/src/pages/CloudFirstWatch.tsx` |
| Assistant contract and views | `internal/mcpcontract`, `internal/mcpserver`, `internal/control`, `web/mcp-app/src/Workspace.tsx` |
| Hosted identity and policy | `internal/cloud/mcp.go`, `internal/cloud/mcp_connections.go`, [cloud MCP plan](mcp-cloud-plan.md) |
| Authoring and packaging | `plugins/shared/skills/ding-watch`, `plugins/chatgpt/ding`, [integration qualification](../integrations/release.md) |
| Delivery and persisted state | `internal/delivery`, `internal/store`, `internal/watchrun`, `internal/serverstate` |
| Research protocol | [Existing adoption pilot](adoption-pilot.md) |

The checkout contains ongoing cloud, push, Console, and store work. Inspect the
current branch before each increment; coordinate with those changes and stage
only the increment's files or hunks. Existing plans record findings at earlier
revisions: verify each gap before implementing a fix.

Keep local use account-free. Do not require billing, marketplace acceptance,
watch transfer, or MCP Events for the first-watch pilot. Initial hosted onboarding
creates a watch directly in its target workspace; it does not require a combined
local/cloud adapter. Broader source catalogs and log collection stay in their
existing workstreams.

## 1. Make “watch the service I shipped” complete

**Deliverable:** one consistent HTTP starter recipe, usable through Console and
the assistant, with a clear first-observation and notification experience.

- Reuse the existing policy: three consecutive HTTP 5xx responses fire; two
  nonmatching responses recover; transport failures are separate source errors
  and do not silently clear an incident. Say “5xx server errors” rather than
  “all failures.” A 401/404 is not proof of service health. Honor an explicitly
  requested expected-status rule using the existing typed comparisons.
- Derive rule descriptions from the compiled definition. Display actual cadence
  and timeout: current starters use 30-second local and five-minute hosted
  polling. Show the resulting detection delay; do not imply instant detection.
- Reuse the existing preview/apply and destination test paths. Show waiting,
  first observation, source error, active incident, and delivery failure distinctly.
  Confirm the first real sample after activation instead of treating apply as
  evidence that acquisition succeeded.
- Keep credentials in protected setup. Allow an existing destination reference;
  guide a new user to the existing secure destination setup and back to the watch.
- Add a controllable synthetic endpoint for qualification with 200, 503, and
  timeout modes. It must be isolated from a participant's production service.
  Supply a short demo script and one focused “watch a shipped service” guide.
- After the first useful observation, offer “Create another watch.” For a local
  watch, explain hosted continuity and eligibility through the existing flow.

**Acceptance:** a new user can configure an owned endpoint without editing YAML;
the synthetic sequence produces one firing event and one recovery event; evidence
and delivery attempts are reachable; source errors remain distinguishable. A
restart preserves incident state. Closing the assistant does not stop monitoring;
only hosted or always-on-server execution is advertised to survive laptop sleep.

**Verification:** focused onboarding/compiler/runtime tests; browser coverage for
the first observation and delivery error states; a real received notification.
Target five minutes to activation for the supported path, recording connection
and account setup separately. Record first-observation time separately because
the hosted cadence can add five minutes.

## 2. Complete the journey in ChatGPT

**Deliverable:** connect → describe → review → activate → inspect → explain, on a
real supported ChatGPT surface and a qualified hosted workspace.

- Verify and close the relevant hosted MCP gaps from [Phases 1–3](mcp-cloud-plan.md):
  effective permissions, execution/workspace identity, consistent payload limits,
  tenant admission, configured OAuth discovery, and full HTTP-path qualification.
  Use one explicitly bound workspace initially. Reject ambiguous targeting;
  broader workspace discovery can ship separately.
- Qualify real login, consent, reconnect/refresh, revoked access, and denied writes.
  Browser login alone must never be described as an MCP connection. Identify an
  owner for the provider configuration, hosted endpoint, and test account.
- Update the shared skill and tool descriptions for the service-watch journey.
  Discover capabilities, use protected setup for missing credentials, preview the
  exact rule, apply within existing user authorization, and verify committed state.
  Preserve operation keys after uncertain writes and reject stale previews.
- Make the embedded view and text fallback agree on execution location, cadence,
  current state, evidence time, and delivery status. An explanation must distinguish
  recorded facts from hypotheses and current readings from event-time evidence.
- Record a reproducible host transcript/screenshots with synthetic data, versions,
  and dates. Include private endpoints rejected by cloud, unavailable credentials,
  interrupted apply, stale review, and unavailable embedded UI.
- Prepare the public listing copy and reviewer journey from that working demo.
  Submission is a separate action; manual pilot installation does not imply
  marketplace acceptance.

**Acceptance:** a fresh participant completes the journey with no pasted bearer
tokens or hand-edited MCP JSON. A real received alert can be explained from its
event ID. Disconnecting/revoking the integration stops its access while the watch
continues. Every advertised client behavior has real-host evidence.

## 3. Run a ten-engineer pilot

**Deliverable:** a small, consent-based study with a repeatable onboarding script,
restricted outcome record, and decisions at days 7 and 30 after each activation.

- Prepare recruitment copy for engineers operating an owned HTTP service and
  already using an assistant. The founder recruits ten participants through
  separately authorized outreach. Prioritize the qualified path; record client,
  execution location, OS, and any assisted setup instead of implying broad coverage.
- Observe the first session. Separate installation/connect time, watch activation,
  first real observation, and notification receipt. Record each intervention.
  A synthetic demo alone does not count as activation.
- Extend the existing pilot record with second-watch date, whether creation was
  independent, use-case category, usefulness at follow-up, coarse support time,
  hosted interest, offer shown, payment outcome, and reason for declining.
  Do not record endpoint URLs, secrets, payloads, or model transcripts.
- A second useful watch monitors a distinct real condition the participant says
  matters. Duplicated demos and researcher-created watches do not qualify.
- Ask at day 7 whether the watch remains useful, what checking it replaced, and
  whether the person has created another. At day 30 ask what they kept, removed,
  or replaced, and whether an alert changed an action. Quiet useful watches count.
- Record missing replies as unknown. Report enrolled, activated, contacted, and
  responding denominators separately; keep local/server/cloud outcomes visible.
  Store contact mapping separately from participant codes and delete on withdrawal.

**Decision rules, fixed before recruitment:** aim for at least 8/10 to activate a
real watch, 5/10 to create an independent second useful watch by day 30, and at
least three actual paid hosted customers from this cohort or a clearly labeled
extension cohort. Report payment conversion over people offered eligible hosting.
These are hypotheses, not release gates. Low activation prompts onboarding fixes;
activation without repeat use prompts use-case interviews; repeat local use with
little paid demand supports a useful tool but leaves the hosting business unproven.

## 4. Offer paid hosted continuity

**Deliverable:** a concrete, manually operated paid pilot before automated billing.

- Draft an initial offer of **$10/month for up to five public HTTP watches checked
  every five minutes**, subject to measured capacity and the operator's cost review.
  This is a test price, not an established willingness-to-pay result. Specify actual
  evidence retention, notification destinations, usage limits, and support before
  showing the offer; do not promise unlimited usage or an unqualified SLA.
- Present it after a useful watch or an explicit request for laptop-off execution.
  Check endpoint eligibility before account creation or transfer. A private service
  on a sleeping laptop cannot become reachable by moving its watch to cloud.
- Reuse direct cloud setup and existing qualified Console/CLI transfer where
  appropriate. Keep a manual entitlement/payment ledger for the small cohort and
  use a separately configured payment or invoicing method. No custom checkout,
  automated subscription engine, or new billing provider is required for this test.
- Define the offer start/end, explicit payment authorization, cancellation, and
  a visible grace/end-of-service notice. Provide export and local/self-hosted
  options before hosted monitoring ends; do not silently delete evidence or stop
  a paid watch. Local rights remain independent of payment status.
- Record offer acceptance, actual payment, renewal when due, refunds, support
  minutes, and infrastructure/delivery cost per paid workspace. Separate stated
  interest, free use, payment, and renewal. Use measured costs to revisit the price.

**Acceptance:** the operator has a costed offer and functioning collection method;
a paying participant receives a real hosted alert with their laptop off; usage and
service end are understandable. Failed payments can be handled manually without
changing local functionality. Automate billing only after observing paid demand.

## 5. Demonstrate event-triggered investigation

**Deliverable:** in a supported ChatGPT surface, “When this watch fires, inspect
the evidence and explain what failed” produces an authorized, bounded response.

Current [OpenAI MCP Events documentation](https://developers.openai.com/plugins/build/mcp-events)
supports webhook subscriptions in Work chats on web, desktop Work with Cloud
selected, and dots, subject to workspace controls. Recheck compatibility during
implementation. The existing integration plan's claim that only protocol
`2026-07-28` works needs correction: current docs also describe initialization-based
Streamable HTTP. Do not force a protocol migration without verifying the pinned SDK.

- Run a small host-compatibility spike first: discover, subscribe, verify callback,
  deliver a synthetic event, and observe the instructed follow-up in the real host.
  Record access constraints before implementing the full persistence path.
- Add `events/list`, `events/subscribe`, and `events/unsubscribe` at the existing
  authenticated MCP boundary. Start with firing/recovered events filtered to an
  authorized watch. Define explicit subscription permission and consent; an inspect
  grant alone does not authorize ongoing outbound delivery.
- Persist owner/workspace/grant binding, canonical subscription identity, watch
  filter, protected callback/signing material, verification state, and expiration.
  Creation/refresh must be idempotent. Verify callbacks before sending evidence;
  validate public HTTPS destinations with the existing egress protections.
- Commit delivery intents with durable events, or use a durable sequenced dispatcher
  with tested restart/replay semantics. Preserve stable event IDs across retries,
  bounded backoff, terminal outcomes, and inspectable delivery state. Recheck access
  at delivery and stop on unsubscribe, expiry, or revocation. Expose actual replay
  support; do not advertise unsupported host gap/control notifications.
- Send minimal event identity and context; retrieve authorized retained evidence
  through existing MCP tools. Keep callback acceptance separate from observed host
  investigation completion. Avoid promising exactly-once model execution.
- Keep initial follow-up read-only: explain observations, rule, timestamps, delivery,
  and possible next checks. Source text remains untrusted data. Add per-subscription
  rate limits and loop protection; host instruction/controls bound model work.
  Remediation requires its own authorization and is outside this demonstration.
- Update the shared skill's current “cannot wake this chat” statement conditionally
  on actual host/event capability. Ordinary webhook/Slack/Discord alerts continue
  independently. Do not claim equivalent Claude wakeups without separate evidence.

**Acceptance:** the controlled endpoint fires, the subscribed ChatGPT conversation
responds with evidence and clearly labeled uncertainty, and recovery is visible.
Tests cover duplicate delivery, restarts, ordering, expiry, revoked membership,
cross-workspace isolation, callback failure, and retained-history gaps. Unsubscribing
stops follow-up without deleting the watch. A real-host run demonstrates behavior
beyond mocks; if host access blocks it, record that blocker and continue steps 1–4.

## Sequence, ownership, and verification

Planning estimate for one implementation owner: 3–5 working days for step 1,
5–8 for step 2, 1–2 for pilot preparation, 2–3 for the paid offer and operations,
and 5–8 for step 5 after its compatibility spike. Re-estimate after the first
increment; OAuth/provider access and real-host behavior can extend calendar time.
Day-7/day-30 research and monthly renewal are elapsed-time observations, not
engineering work or prerequisites for shipping fixes.

| Order | Small, independently reviewable increments | Owner / dependency |
| --- | --- | --- |
| A | Starter rule descriptions and related tests; first-observation states and tests; controlled demo and guide | Engineering; start here |
| B | Verified hosted capability/policy fixes, one behavior per commit; OAuth host qualification; shared skill journey; embedded/text parity | Engineering plus account operator; after A for complete demo |
| C | Pilot script and outcome schema; first three sessions; fix observed friction; remaining seven sessions | Founder/research owner; start after A, add assistant sessions after B |
| D | Costed paid offer; manual payment/entitlement runbook; continuity prompt and related tests if needed | Founder/operator plus engineering; after real activation and hosting qualification |
| E | Host event spike; subscription migration/repository; authorization and lifecycle; verified signed delivery; durable dispatch; investigation skill and real-host evidence | Engineering plus host account owner; after B, alongside pilot follow-ups |

Commit each coherent behavior with its directly related tests after verification,
before starting the next independent behavior. Preserve those commits. Do not
combine the plan's work with unrelated cloud or Console edits.

Use focused Go tests for touched onboarding, control, cloud, MCP, store, and runtime
packages. For UI changes run the relevant unit/browser tests and build. Use
`make test-mcp` for integration milestones and the existing full tagged Go/race/vet
checks for release qualification. Record real OAuth, notification, laptop-off, and
host investigation checks separately from automated test results.

The two 24-hour stdin/NDJSON and followed-file/metrics soak scenarios remain
optional, non-blocking background validation. Preserve results and report actionable
failures separately. They must not become required CI statuses, release/deploy
dependencies, or per-release waivers. Pilot retention and renewal observations
likewise do not delay continuous delivery.

Execution begins with increment A: align the service-watch rule descriptions and
verify the existing local and cloud first-watch behavior before extending the UI.

## Implementation record — November 5, 2026

The first source increments are committed separately:

- `307e390`: local and cloud previews render descriptions from the compiled rule,
  including cadence, timeout, source errors, and time between matching checks.
- `55ccf07`: after activation, the Console confirms actual monitoring input,
  distinguishes source/delivery problems, and offers a fresh second-watch setup.
- `4964516`: a loopback-only development fixture exposes healthy, failing, and
  timeout modes without changing a user's service.
- `c39c2c5`: both starter recipes are exercised through real HTTP acquisition and
  receipt, persisted restart, incident recovery, and verified retained evidence;
  the [service-watch guide](../guides/watch-shipped-service.md) documents the flow.

The focused Go packages pass race tests and vet, all 34 Console unit tests pass,
and all four Chromium onboarding tests pass, including the real scheduler demo.
The Console production build passes. These checks use the current checkout,
which also contains pre-existing uncommitted work; those changes were not included
in these commits.

Step 1 remains open for direct reuse of existing destinations in the starter
form and participant/device qualification. Local network destinations currently
use Workbench, and expected-status customization uses its existing typed rule
editor. Human-visible native notification receipt and deployed cloud/laptop-off
behavior have not been established by these local automated checks. Steps 2–5
retain the dependencies and acceptance criteria above.
