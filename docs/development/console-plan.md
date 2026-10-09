# Ding Console: product, interface, and implementation plan

Status: evidence-first direction selected by the user; implementation plan proposed. October 9, 2026. Audited against commit `3db426f` and the actual Cobra command tree. This document plans the console; it does not claim that these screens or new APIs exist. The current watch runtime and its qualification run remain separate work.

## Product promise

Open Ding and understand what is being watched, what happened, why it happened, and whether the notification arrived. Create or change a watch with a clear preview of its behavior and its effect on existing state. Every supported CLI operation must have an intentional home in the interface, with the host-process exceptions described explicitly below.

The distinctive experience is the connection between **observation → decision → event → delivery**. A watch page should make that chain tangible. A person should be able to answer “Why did this alert fire?” without reading JSON, then inspect and export the exact evidence when needed.

Ship an open source console with the existing Go executable. Target a developer running one Ding daemon locally or behind their own authenticated TLS deployment. The first console does not require a Ding account, a cloud service, an LLM, or an additional production Node process.

## Design references and our interpretation

| Reference | Useful pattern | Application to Ding |
| --- | --- | --- |
| [Temporal Web UI](https://docs.temporal.io/web-ui) | Searchable executions, detailed history, human and raw views, contextual actions | Searchable watches; durable event history; inspect evidence and control a watch in context. Use Ding's vocabulary and state model. |
| [Linear's interface refresh](https://linear.app/now/behind-the-latest-design-refresh) and [filters](https://linear.app/docs/filters) | Quiet navigation, consistent headers, information density, filters reflected in URLs | A restrained shell, compact readable watch lists, keyboard navigation, and restorable filtered views. |
| [Sentry issue details](https://docs.sentry.io/product/issues/issue-details/?promo_name=hp-banner) | An issue summary connected to a specific event and its supporting context | An understandable event headline followed by its condition, retained inputs, source state, and delivery results. |
| [Stripe Workbench](https://docs.stripe.com/workbench/overview) | Related events, payloads, and delivery attempts in one investigation flow | Follow an event into a destination attempt without losing the selected watch or list position. |

These are interaction references. The design direction below is our proposal, not a claim that another product implements Ding's semantics.

## The visual direction

Call the interface **Ding Console**. Preserve the existing green brand, but give the product a quieter, more precise presentation than the terminal-styled marketing site.

- **Surfaces:** warm near-white in light mode; graphite in dark mode. A slightly recessed navigation rail and a clear main workspace. Theme follows the system, with an explicit preference.
- **Color:** a small amount of Ding green for the brand and primary actions. Separate named semantic tokens for success, active incident, attention, unknown, and paused. Always pair color with text or a symbol. Selected navigation uses a neutral surface, so it does not look like a success signal.
- **Typography:** a readable sans-serif for the product, monospace for IDs, values, timestamps, and definitions. Proposed scale: 28px page titles, 18px section titles, 14px body/table text, 12px metadata. Self-host any chosen fonts; use system fallbacks. No remotely fetched fonts at runtime.
- **Geometry:** 4px spacing base; 8/12/16/24/32px rhythm. A roughly 208px desktop rail, 56px location bar, and 24–32px content inset. Borders organize tables and sections; modest radii on controls and bounded panels. Reserve shadows for overlays.
- **Density:** comfortable rows around 48px, optional compact rows around 36px. Preserve a readable text size when increasing density. Keep important values aligned and use tabular numerals.
- **Motion:** short, purposeful transitions for expansion, selection, and changed values; honor reduced motion. Live updates do not continuously pulse or reorder the item someone is reading. No automatic notification sound.
- **Depth:** the first level is a plain-language explanation, the second is structured evidence, the third is the original data. JSON is consistently available through a “Raw data” action.

The signature component is a small four-stage evidence strip: **Observed / Evaluated / Recorded / Delivered**. Each stage shows its actual outcome and opens supporting detail. It must also work when only part of the chain exists: no event yet, lifecycle event without replay evidence, no configured delivery, pending delivery, or unavailable history.

### Selected layout: evidence first

The user selected **evidence first** after reviewing the two interactive concepts. The watch detail leads with its current condition and an explanation of the selected event. A timeline and inspection panel sit beneath it. This makes a small watch installation approachable while retaining depth.

Continue refining this layout with real evidence and failure cases during U01. The timeline-first concept remains a design reference; it is not an additional implementation requirement or a layout toggle for the first release.

## Information architecture

| Navigation | Primary job | Core content |
| --- | --- | --- |
| **Watches** — default landing page | Know what needs attention; open or create a watch | Search, filter views, watch list, condition/source/delivery state, latest input, primary action “New watch” |
| **Events** | Understand what happened | Retained history, watch/type/time filters, live follow, event inspector and evidence |
| **Deliveries** | Find notifications that need intervention | Pending, sending, delivered, failed and canceled deliveries; attempt history and retry |
| **Workbench** | Create, understand, and prove a definition | YAML editor, structured explanation, fixture tests, dry-run review, apply, evidence replay, legacy import |
| **System** | Diagnose and operate this daemon | Doctor, limits, retention, credential presence, destinations, backup, runtime and console versions, connection details |

Avoid a separate dashboard whose main purpose is a row of counters. The Watches page can carry one concise attention summary, and every count links to the corresponding filtered list. Counts must come from complete server queries, never the currently loaded page.

The shell identifies the connected daemon and its freshness at all times. V1 connects to the serving daemon; it does not imply a fleet, organizations, or environments that Ding does not have. Use a stable instance identity in local saved-view keys to keep two local daemons' preferences separate.

### State language is part of the design system

| Dimension | Examples | Presentation rule |
| --- | --- | --- |
| Watch lifecycle | Running, paused, deleted | Says whether new acquisition/evaluation is enabled; it does not summarize the condition or delivery queue. |
| Condition / incident | Waiting for input, no incident, matching 2 of 3, incident open, recovery 1 of 2, unknown | An unknown evaluation may retain an open incident. For level/change/new-event policies, use their actual evaluation/event vocabulary rather than inventing an incident. |
| Source health | Receiving, acquisition error, freshness unknown | A successfully received HTTP 503 can satisfy a condition while acquisition is healthy. An old push input is not automatically a source error without a defined freshness/deadline rule. |
| Delivery outcome | None configured, queued, sending, retry scheduled, delivered, permanent failure, exhausted, canceled | An event can be recorded successfully while delivery has failed. Use the intent's status and recorded attempt data. |
| Console connection | Connected, refreshing, reconnecting, disconnected | This describes the browser's knowledge, not the daemon's actual execution state. Show the last successful read. |

Grouped watches may contain mixed entity states. Show complete counts and a route to the affected entities instead of assigning the first loaded entity's status to the entire watch. Stable icon/text/color mappings apply everywhere.

### Routes and navigation rules

Use stable routes such as `/ui/watches`, `/ui/watches/:id`, `/ui/events/:id`, `/ui/deliveries/:id`, `/ui/workbench`, and `/ui/system`. Keep filters and selected tabs in URL parameters. A contextual inspector can open over a list on a wide screen; its deep link must also work as a full page. Back restores search, scroll position, and selection. Never put manifests, payloads, credentials, or evidence in URLs.

At laptop widths keep the list/detail relationship when it fits. At tablet widths collapse the rail and move side detail below its parent. At phone widths use a full-page inspector and explicit back navigation. Tables may scroll in their own region when necessary; controls and explanations must still reflow. Validate 375, 768, 1024, and 1440px layouts, plus 200% zoom.

## The important screens and journeys

### 1. Watches: useful within five seconds

Default columns: watch name and ID; lifecycle; condition; source; delivery attention; latest accepted input. Source type and interval belong in secondary text or an optional column. Use an actual watch name where supplied and always make the stable ID accessible. Long names and entity keys must not make controls disappear.

Default views are **All**, **Needs attention**, and **Paused**. Needs attention includes open incidents, source errors, absent required credentials, and terminal delivery failures, each with its own reason. “Waiting for first input” is a first-class state. A new or paused watch does not become “Healthy” merely because it has no alert.

Search IDs and names. Add server-backed filters for lifecycle, source type, incident state, source health, and delivery state. Saved views are browser preferences, scoped to the daemon. No arbitrary query language is needed initially. The page offers a guided example when the store is empty, and a distinct “No matches” result when filters hide existing watches.

Row selection opens the watch; a clearly labeled menu holds infrequent actions. Keep Delete away from Pause. Do not put five action buttons on every row or make a tiny status dot the only way to inspect an error.

First use starts with three concrete choices: inspect an HTTP endpoint, receive pushed JSON, or run a local command. Each offers an editable supported template, describes what will execute on the daemon host, and shows any missing environment references before review. A sample fixture demonstrates the rule before the first apply. After apply, land on the new watch with “Waiting for first input” and the next expected poll or push integration instructions. Example data lives in an explicitly labeled preview, never mixed into the user's real watch list.

### 2. Watch detail: the defining experience

The top of the page answers:

1. **What does this watch do?** “Check this endpoint every 5 seconds. Open an incident after 3 consecutive responses with status 500 or above; recover after 2 nonmatching responses.” This comes from the compiled definition, with exact semantics available on expansion.
2. **What is its state now?** Separate lifecycle, condition/incident, acquisition health, and delivery outcome. Show when that snapshot was read.
3. **Why did the selected event occur?** Show the event's revision and evidence at that time. Never explain yesterday's event using today's edited definition.
4. **What happened next?** List its delivery intents, attempts, and next scheduled retry where present.

Tabs: **Overview**, **Events**, **Entities**, **Deliveries**, and **Definition**. Hide the Entities tab only when the definition cannot produce multiple entities; keep the underlying ungrouped state available in Overview. The Definition tab includes applied YAML, revision, permissions, destination references, export, and “Edit in Workbench.” Show historical event definitions through the event inspector. A full revision browser and rollback are outside the initial parity scope.

Example headline: “API returned a server error on the third consecutive check.” Below it: newest value `503`, rule `http.status ≥ 500`, prior matching count `2`, resulting count `3`, and event `firing`. If all three raw observations are retained, make them inspectable. If only the replay checkpoint and latest input are available, label the prior count as checkpoint state; do not manufacture a three-request trace.

A second example: “No observation arrived before the deadline.” Show the last accepted input, configured missing interval, logical deadline, and timer evidence. A third: “A new provider event was seen.” Show event ID and deduplication horizon. These need different evidence presentations; a numeric chart is not universal.

For numerical window conditions, show quantity, units if defined, aggregation, window boundaries, sample count, and coverage gaps. Never interpolate categorical HTTP codes into a smooth trend or imply measurements exist outside retained samples. Distinguish source timestamps, accepted timestamps, logical time, and display timezone. Surface clock resets and retention gaps.

### 3. Events: a readable chronology

Use short, concrete rows such as “Incident opened,” “Source stopped returning observations,” “Watch paused,” and “Condition state reset.” Retain the exact event type and ID in the inspector. Group by day and allow local, UTC, or relative time display; absolute timestamp is always available.

An inspector contains the human explanation, selected fields, original event, relevant definition revision, replay status, and related deliveries. Evidence and observations use independent pagination. Large JSON loads only on demand, has bounded rendering, and offers a download.

Live follow is explicit. Pausing **live updates** does not pause a watch; labels and placement must make that unambiguous. When the user scrolls away from the newest event, show “N new events” instead of pulling the viewport. After disconnect, resume from the cursor. A `410 cursor_expired` creates a visible history-gap message and an action to load available history; do not silently present continuity.

The current API reads forward from a retention-aware cursor. A newest-first initial view and older-history paging require additional read APIs. Do not fetch and reverse all events in the browser.

### 4. Workbench: understand before applying

Use an editor and an adjacent preview at desktop widths, stacked on smaller screens. The preview is a readable plan: source, selected fields, condition, trigger/recovery behavior, permissions, limits, and destinations. Multi-document bundles show every watch and destination. The editor is authoritative and supports every existing schema feature.

The creation flow is **Choose example or import → Edit → Validate/explain → Test if a fixture is available → Review changes → Apply**. Validation is automatic after a short debounce, plus an explicit command for keyboard users. Applying always validates again on the server. Fixture testing is available without being a mandatory obstacle for a simple new watch.

Initial templates use supported HTTP, command, and push sources. Guided fields may cover these common templates; any form-to-YAML edit must preserve other valid fields and comments, or explicitly switch to YAML-only editing. Never silently discard a valid advanced definition. No natural-language creation endpoint is assumed.

Errors belong at the affected field or line, with a concise explanation and remediation. General compiler errors remain visible even when no reliable source location exists. Diagnostics need stable codes, severity, field path, and optional line/column ranges from Go. Browser syntax assistance is advisory; the Go compiler owns validity.

Tests accept JSONL and an explicit watch selection when needed. Show a timeline of predicted events, observations processed, and explanatory evidence. Clearly label it **Simulation**. Tests do not run sources or send notifications. Replay is separately labeled **Verify recorded evidence**, with verified, mismatch, unsupported/unavailable, and malformed-input outcomes.

**Review changes** is a dedicated screen, not a small confirmation dialog. Show a semantic summary and text diff for each definition, old/new revisions, new watches, state preserved versus reset, destination changes, missing environment references, and command/network execution permissions. Show the actual daemon receiving the change. Capture the reviewed versions and invalidate the review if the draft changes. A conflict preserves the draft and offers “Compare with current”; it never silently force-applies.

The existing apply response reports watch changes but does not fully describe destination-only changes or guard concurrent destination edits. Extend the shared apply contract before promising a trustworthy whole-bundle review. Check all relevant watch and destination revisions atomically when committing; retain compatibility for existing CLI clients.

Unsaved drafts stay in memory by default. Provide explicit download; do not silently persist possibly sensitive manifests or fixtures to browser storage. Preserve a draft across reauthentication in the same page session. Confirm navigation away only when it would discard edits.

### 5. Deliveries: make failure actionable

Use a list with destination, related watch/event, status, attempt count, latest outcome, and next retry. Translate statuses without losing the raw value: `pending` means queued or waiting for a retry, `leased` means sending, `permanent` and `exhausted` are different terminal failures, and `canceled` is explicit cancellation.

Show attempt history with time, outcome, and recorded detail. A returned source HTTP 503 and a destination HTTP 429 must appear in different places with different explanations. Only show an HTTP status if it was actually recorded, rather than deriving it from an imagined transport log.

“Retry delivery” appears for eligible terminal states and explains that it resends the original event to its recorded destination revision. Existing receipt may be uncertain, so duplication is possible. Require a focused confirmation for that action and disable repeat submission while awaiting the response. Do not show “Retry now” for an already scheduled pending item when the API cannot perform that operation.

### 6. System: diagnostics a person can act on

Present Doctor as named checks with result, last checked time, explanation, and an appropriate next action. Group runtime, storage, sources, credentials, resource limits, and deliveries. A credential view shows environment variable names and presence only. It must not fetch their values into the browser.

Keep live connection/freshness separate from the latest full diagnostic result. `doctor` currently runs storage health checks; do not call it every second to paint a green indicator. A lightweight status endpoint serves that purpose. Run full diagnostics on entry or explicit request with bounded concurrency.

Destinations are browsable here, including where used and current revision. Editing opens a manifest in Workbench and follows the same apply review. No separate mutation path bypasses validation.

Backup offers **Download verified backup** and an advanced **Save on daemon host** option matching the CLI. The latter must say which host and path will receive the file and retain no-overwrite behavior. Browser downloads use a bounded managed temporary artifact and authenticated download route, with cleanup on completion, expiry, failure, and daemon restart. Do not let a download endpoint read arbitrary server paths.

Legacy import takes uploaded YAML, shows converted/unsupported items and required secret bindings, and downloads the generated files plus report as an archive. It never starts the converted watches. The user can explicitly bring a converted manifest into Workbench afterward.

## Complete CLI parity inventory

The inventory includes automatically generated Cobra help/completion commands. “Existing” below means backend behavior exists, not that a screen exists.

| CLI command / meaningful option | UI equivalent | Required backend work |
| --- | --- | --- |
| `validate FILE [--json]` | Workbench validation and diagnostics; raw result | New pure-operation endpoint using `plan.Parse` |
| `explain FILE [--json]` | Readable definition preview, capabilities and permissions | Shared compile/explain contract; deterministic presentation model |
| `apply FILE --dry-run` | Review changes and semantic diff | Existing apply dry run; extend complete bundle/destination review |
| `apply FILE --expected-revision REV --watch ID` | Apply reviewed bundle with concurrency checks | Existing expected watch revisions; add destination preconditions and review consistency |
| `watch list` | Watches page | Existing list; add bounded summary query and filters |
| `watch inspect ID --entities-after KEY --deliveries-before N` | Watch detail and independently paginated Entities/Deliveries tabs | Existing inspection plus complete aggregates and useful evidence links |
| `watch pause ID --expected-revision REV` | Pause watch action | Existing lifecycle API; refresh and show committed state |
| `watch resume ID --expected-revision REV` | Resume watch action | Existing lifecycle API; refresh and show committed state |
| `watch delete ID --expected-revision REV [--cancel-pending]` | Delete with explicit pending-delivery choice | Existing lifecycle API; explain retained history and uncertain in-flight receipt |
| `events --watch ID --cursor C --limit N --follow` | Filtered Events page, paging, resume cursor, live follow | Existing forward cursor API; add newest-first/query read model |
| `events inspect EVENT_ID` | Event inspector, replay status, evidence JSON download | Existing evidence API; add lightweight summary separate from heavy proof retrieval |
| `events observations EVENT_ID --after N` | Selected observations table with “Load more” | Existing paged evidence-input API |
| `delivery inspect ID --before N` | Delivery detail and paged attempts | Existing inspection; add global delivery listing |
| `delivery retry ID` | Retry eligible terminal delivery | Existing retry API; contextual confirmation and error states |
| `doctor` | System diagnostics | Existing full doctor; new cheap status summary |
| `export --watch ID` | Download applied YAML, including referenced destinations | Existing export API; clearly identify current destination definitions |
| `backup --out FILE` | Download verified backup or save to new path on daemon host | Existing path backup; add managed artifact download |
| `test FILE --events FIXTURE.jsonl --watch ID` | Fixture test runner | New bounded upload endpoint calling `replay.Run` |
| `replay EVIDENCE.json` | Evidence verification tool | New bounded endpoint calling `replay.Verify` |
| `migrate --config OLD.yaml --out NEW_DIRECTORY` | Legacy import, conversion report, generated-file download | Expose pure converter and archive output; browser chooses download location |
| `version`, `--version` | System About: binary, API, schema and UI build versions; raw data | New version/capability response |
| `daemon`, `--state-dir`, `--listen`, `--allow-remote`, quotas, `--history` | Instance configuration view and launch/setup instructions | Expose effective non-secret settings; host startup remains CLI/service-manager work |
| `help [COMMAND]`, `--help` | Contextual help and searchable command reference | Bundle generated reference from the Cobra command tree |
| `completion bash/fish/powershell/zsh` | System → CLI setup → download shell completion script | Generate from the same command tree; no shell execution |
| `--json` throughout | Raw response / copy / download alongside the readable view | Preserve versioned envelopes; redact no data silently |
| Hidden legacy `run`, `serve`, `test-rule`, `install` | Legacy migration guidance with old release link | Match existing tombstone guidance; do not recreate removed runtime behavior |

**Process boundary:** a browser page served by a stopped daemon cannot launch that daemon. V1 must be honest about this. Existing `ding daemon` still starts the process; propose `ding ui` to open and authenticate to an already running instance. The UI can show and help prepare startup settings, but does not silently modify them or promise restart/start buttons. Literal browser-only process management would require a separate supervisor or desktop application and is a separate product decision. All ordinary operational commands above are executable from the UI after connection.

Pause stops acquisition and timers; committed deliveries continue. Delete retains history, and pending deliveries continue unless cancellation is explicitly selected. The interface must communicate those effects at the point of action. Avoid an optimistic success toast before the server commits.

## Architecture and API work

### Repository structure: one repository, distinct build and deployment boundaries

Recommend keeping the Go runtime, console, public website, and documentation in the Ding repository. The public website already lives in `workers/website`, and documentation source lives in `docs`. Introduce the console at `web/console`; keep the existing Go package structure and website location rather than reorganizing unrelated code to accommodate the frontend.

| Component | Source | Build and release boundary |
| --- | --- | --- |
| Runtime and CLI | `cmd/ding`, `internal` | Go executable, containers, existing platform releases |
| Operational console | `web/console`, `internal/webui` | Static assets built and embedded into the matching Ding executable; served by that daemon |
| Public website | `workers/website` | Independently deployed Cloudflare Worker/static site for `ding.ing` |
| Product documentation | `docs`, `cmd/docgen`, `mkdocs.yml` | Independently published documentation; generated CLI reference stays aligned with the source |
| Install endpoint | `workers/install` | Existing independent deployment |

One change can update the runtime contract, generated frontend types, UI, examples, and documentation together. Share the brand assets and small design-token definitions where useful. Share runtime semantics through the Go API and generated contracts. Extract a shared UI package only when both consumers actually need the same components; marketing pages and operational screens have different interaction requirements.

Use ordinary Go and npm builds with scoped CI. Console-enabled releases build and test the matching frontend assets. Runtime/API changes trigger Go, contract, and relevant console checks; console changes trigger frontend checks and binary embedding smoke tests; website-only changes run website checks and deployment. Documentation checks must also respond to changes in generated CLI/API inputs. Keep deployment credentials and workflows scoped to their component. A public-site deployment does not release the daemon, and the public site does not receive local daemon credentials.

Reconcile the existing website/docs deployment work before editing those workflows. The primary checkout currently has in-progress website workflow and `workers/docs` hosting files; preserve them and establish the intended documentation deployment path instead of introducing a competing publisher. This repository recommendation does not itself move files or change hosting.

### Preserve the runtime boundary

The browser talks to the Go control API, which calls the same application/compiler/replay/migration packages as the CLI. No subprocess execution of CLI text, no direct browser/database access, and no JavaScript implementation of the condition evaluator. Human explanations are deterministic presentations of definitions and evidence; runtime behavior requires no model call.

Proposed layout:

```text
web/console/                 TypeScript application and browser tests
  src/app/                   Shell, routes, connection and preferences
  src/features/              Watches, events, deliveries, workbench, system
  src/components/            Shared interaction and evidence components
  src/api/                   Generated contracts and request client
internal/webui/               Embedded static assets and SPA routing
internal/control/            API, browser sessions, bounded tool endpoints
internal/plan/                Structured compiler diagnostics / explanation
internal/store/              Bounded read models, indexes if measured necessary
docs/development/             Design contract, parity inventory, phase evidence
```

Use [React with TypeScript](https://react.dev/learn) and [Vite](https://vite.dev/guide/) for a static application, [TanStack Query](https://tanstack.com/query/latest/docs/framework/react/overview) for server state, and accessible unstyled [Radix primitives](https://www.radix-ui.com/primitives/docs/overview/introduction) for interactions. Use a route library for nested/deep-linked screens, a lazily loaded YAML code editor, and custom CSS tokens/components. Evaluate CodeMirror against the editing and accessibility requirements during the design spike; choose and pin the actual editor version then. Avoid adopting a pre-styled admin theme as the design system.

Embed production assets in the Go release binary. Frontend tooling is a contributor/build dependency. Provide a documented headless build target, reproducible locked dependency install, development proxy, and a clear error if a console-enabled build lacks assets. Extend release/container CI to build and embed assets on every supported target. Keep source-only Go tests and CLI development usable without installing Node. Do not merge this with the marketing site's Cloudflare deployment.

### Additive contracts

Route names below are proposed. Freeze them with request/response schemas before implementation.

| Proposed contract | Purpose and constraints |
| --- | --- |
| `GET /v1/info` | Version, capabilities, instance identity, effective non-secret configuration, payload limits, server time |
| `GET /v1/status` | Cheap timestamped runtime/connection and attention summary; no database integrity scan on each poll |
| `GET /v1/watch-summaries` | Cursor-based watch summaries, supported filters/sorts, complete counts and snapshot metadata |
| `GET /v1/event-summaries` | Indexed watch/type/time filtering, newest-first initial page, older paging and compatible forward-follow cursor; no replay verification during list rendering |
| Bounded retained-observation query | Supply a selected watch/entity/time slice for the observation timeline where raw observations still exist; identify coverage limits and checkpoint-only evidence. Never fetch all stored observations to draw the screen. |
| `GET /v1/deliveries` | Paged global delivery queue/history with explicit watch/event/destination/status filters |
| `GET /v1/destinations` | Current definitions, references and usage, secret names/presence; distinguish pinned historical revisions |
| `POST /v1/tools/compile` | Validate/explain uploaded manifest, structured diagnostics and deterministic explanation; no source I/O |
| `POST /v1/tools/test` | Bounded manifest + JSONL input; watch selection; cancellation-aware fixture test |
| `POST /v1/tools/replay` | Bounded evidence input; same verifier as CLI; no deliveries |
| `POST /v1/tools/migrate` | Strict legacy conversion, explicit unsupported items, safe generated-file names and downloadable archive |
| Managed backup artifact create/download/delete | Verified backup via existing store code, authenticated bounded output, expiry and no arbitrary-file reads |
| Browser handoff/session create/delete | Short-lived session establishment and logout; separate from existing admin/ingest bearer credentials |

Preserve all existing CLI response shapes and cursor behavior. New read endpoints avoid silently changing `/v1/watches` from an array to a page or changing existing forward event cursors. New cursors encode the instance, query scope, sort, and snapshot boundary. Filters changed in the UI start a new query. Handle concurrent writes, deletions, and retention explicitly. Add indexes only against measured queries; any schema migration retains verified pre-upgrade backup and migration coverage.

Generate a TypeScript client/type contract from a checked-in API schema tied to Go handler types, with CI drift checks and representative request/response fixtures. Existing watch JSON Schema describes definitions, not the entire control API. Do not treat it as an API specification.

Start with adaptive polling: visible event follow at roughly 1s; active watch/list snapshots around 5s; slower system summaries; suspend nonessential queries in background tabs and back off on failure. On focus/reconnect, reconcile current state and cursor. Coalesce queries and do not download heavy proof/JSON on each refresh. Only add SSE if measured need justifies its connection, authentication, and resume complexity.

### Browser access and sensitive operations

Existing API bearer authentication is not a browser session design. Before exposing mutations:

- `ding ui` uses the local credential to request a single-use, short-lived handoff token and opens `/ui/` with that token in the URL fragment. Clear the fragment immediately, exchange it once, then use a same-origin HttpOnly session cookie. Never place the long-lived admin token in a URL or browser storage. Print a one-use link if opening the browser fails.
- Bind sessions to the instance, expire them, invalidate them on daemon restart, and provide logout. Keep ingestion authentication separate; a UI session must not reveal the ingest token or acquire arbitrary ingest access implicitly.
- Require appropriate SameSite cookies, exact origin and Host checks, and CSRF protection for cookie-authenticated mutations. Reject unexpected origins/hosts and test DNS-rebinding cases. Remote browser sessions require explicitly configured HTTPS origins/proxy behavior and Secure cookies; do not accept spoofed forwarded headers by default.
- Serve static assets with correct content types, restrictive CSP and framing policy. Current API middleware sets JSON content type on every path; split static/UI routing carefully so an SPA fallback never swallows API errors or missing assets.
- Treat watch names, entity keys, payloads, and errors as untrusted text. No raw HTML rendering. Escape/cap rendered data; no external scripts, telemetry, or browser storage of secrets by default.
- Applying a command source grants daemon-host execution. Show its executable, arguments, working directory and environment variable names in review. No generic “run a terminal command” browser feature.
- Bound tool body sizes, outputs, CPU time, temporary disk usage, and concurrent jobs. Evidence verification currently accepts up to 64 MiB through the CLI; normal API decoding is capped at 2 MiB. Give tool endpoints explicit separate limits instead of accidentally changing all API limits or base64 inflating large files.
- Test auth expiry, large uploads, canceled requests and unknown mutation outcomes. A timed-out apply must reconcile revisions/state before offering another submission. A timed-out delivery retry must inspect the intent, since automatic replay could schedule an extra cycle.

### Data the UI must not invent

Do not promise historical uptime, arbitrary latency charts, an operating-system process log, every raw HTTP response, or a complete lifetime timeline unless the runtime actually retains the required data. Display the retained horizon, checkpoint provenance, and missing fields. Where a useful view needs a new read model, add it explicitly. Persisting new telemetry requires a separate storage/retention budget and tests, not an unnoticed side effect of adding a chart.

## Interaction quality and state specification

Every primary screen needs designed fixtures for empty, loading, ready, refreshing, disconnected, unauthorized, error, partial data, and expired history. Every write needs submitting, success, rejected, conflict, and outcome-unknown states. Keep last good data visible during refresh with an accurate freshness label; after connection loss, remove “Live” and disable writes until reconnection.

Keyboard operation includes the command menu, search/filter access, list movement, inspector open/close, and all form/dialog actions. Provide visible shortcuts and contextual help. Shortcuts do not fire while editing text. Focus returns to the triggering control after closing an inspector; browser back should restore the selected row. Avoid visual-only affordances and hover-only controls.

Use semantic HTML, visible focus, accessible dialogs and tables, readable contrast, and screen-reader labels. Status changes use restrained live announcements; streaming hundreds of rows must not announce every arrival. Test reduced motion, touch targets, zoom, light and dark themes, long text, huge numbers, many entities, and missing data. UI error messages should say what failed, whether anything changed, and what the user can do next.

## Sequenced implementation and exit gates

Complete each phase and its tests before building on it. Commit coherent working increments within a phase and a phase-completion checkpoint with evidence. Track the inventory above in a machine-readable parity checklist so newly added Cobra commands cannot silently bypass a UI decision. A checkbox requires working behavior and its test, not just a menu item.

| Phase | Deliverables | Required exit gate |
| --- | --- | --- |
| **U01 — Interaction design and contracts** | Screen/route map; refinement of the selected evidence-first prototype; tokens; state fixtures; complete CLI map; repository/build, API and session ADRs | Walk through create/test/apply, explain firing, investigate failed delivery, pause/resume, conflict and disconnection in the prototype. Every required datum is traced to an existing field or an explicit API task. Reconcile the existing website/docs deployment work before adding or changing workflows. |
| **U02 — Console foundation and browser access** | Static build/embedding; app shell; routes; generated contracts; `ding ui`; sessions, origin/CSRF controls; info endpoint | Released binary serves deep links without Node; headless build works; API semantics remain compatible; auth, static routing and security integration tests pass. |
| **U03 — Readable operational views** | Watch summary queries; Watches, detail, Events and Deliveries; evidence inspector; paging and follow; retention/freshness states | Real daemon fixture supports watch → event → observation → delivery navigation. Complete counts, cursor gaps, pagination and disconnected state tested. No unbounded history fetches or per-row proof verification. |
| **U04 — Workbench and reviewed apply** | Compile/diagnostics; editor/explanation; fixtures and test timeline; full-bundle dry run, diff, concurrency protection and apply | Valid definitions apply identically from CLI/UI. Invalid input never runs. All supported operators have honest explanations; destination-only changes and multi-watch conflicts are covered. Editing after review invalidates it. |
| **U05 — Lifecycle and delivery operations** | Pause/resume/delete; explicit pending cancellation; retry; contextual action feedback | Tests prove paused watches can still deliver committed events, deleted watch history remains inspectable, revision conflicts preserve state, and only terminal deliveries retry. Handle unknown write outcomes without unsafe automatic repeats. |
| **U06 — Full parity and first-run experience** | Doctor; export; verified backup/download and host-path option; evidence replay; legacy conversion; destination view; version/help/completion; onboarding | Every parity row is demonstrated against a real daemon or explicitly marked as a host setup boundary. Downloads round-trip through existing CLI tools; migration reports unsupported behavior and never applies automatically. |
| **U07 — Design refinement and release qualification** | Cross-screen visual review; keyboard/accessibility pass; responsive/theme tests; performance budgets; packaging/docs/screenshots | All critical journeys pass in Chromium, Firefox and WebKit; no serious/critical automated accessibility findings and manual keyboard/screen-reader review complete. Production binary/container smoke tests and runtime regression gates pass. |

U01 is substantial work: polished critical screens and failure states, not wireframes handed off with “make it look nice.” U07 refines an already coherent interface. Design decisions and component quality are reviewed in every phase.

### Test strategy

- **Go unit/integration:** read-query boundaries and complete counts; cursor query scoping and retention; pure tool endpoints; cancellation and quotas; destination/watch review preconditions; session and CSRF rules; artifact path safety/cleanup; existing API compatibility.
- **Component tests:** explanation presenters and state labels, diagnostic locations, semantic diff, large/empty result states, form accessibility and URL restoration. Test observable behavior rather than reproducing implementation details.
- **Browser tests with a real Go daemon and temporary SQLite store:** end-to-end creation from all three source types; actual fixture sources and local delivery receivers; incident/recovery evidence; 429 retries and terminal failure; lifecycle and conflict; download/CLI round trips. Stub external providers, not Ding's own API in the critical release tests.
- **Deterministic visual fixtures:** commit screenshots for core screens, attention/empty/error states, both themes, and the target widths. Review changes deliberately; do not blindly regenerate baselines. Fixture data must be clearly separated from a connected runtime.
- **Data scale:** 1,000 watches, at least 100,000 retained events, many entities and attempts, long identifiers, and a large permitted proof. Measure with active acquisition/delivery so the UI cannot starve the runtime. Pagination and expensive-work isolation are release requirements.
- **Provisional performance budgets:** first usable local list within 1s on documented reference hardware with warm assets; local selection/filter feedback within 100ms before network completion; bounded page size of 100 by default; initial JS budget around 250 KiB gzip with editors and large viewers loaded on demand. Validate and adjust these during U02/U03 using measurements, recording any tradeoff rather than silently dropping the budget.
- **Runtime regression:** existing Go/race/platform/container gates continue to run. Compare headless and console-idle resource use, then test with active UI polling. The currently running P14 soak is left intact; changes to serving/query/runtime behavior require targeted qualification on the new code and a renewed release soak when warranted. Old soak results do not automatically qualify a changed binary.

## What makes the release finished

A developer can create a real watch, test it, review and apply it, observe an actual incident, understand the recorded reason, investigate its delivery, pause/resume it, and export or back up their work without needing JSON to make sense of the result. An advanced user can still inspect all raw contracts and use the CLI interchangeably.

Before release, run short task sessions with developers who did not build the UI. Ask them to identify the watch needing attention, explain why an event fired, distinguish a source failure from a delivery failure, preview an edit's state impact, and recover from a revision conflict. Record errors and hesitation. “Why did this fire?” and “Did the notification arrive?” should each be answerable in roughly 30 seconds from the relevant list without reading raw JSON. Treat that as a design target to test, not a measured claim.

Defer fleet management, hosted accounts/billing, AI-generated explanations, provider-specific consumer dashboards, arbitrary browser shell execution, automatic rollback, and a full metrics warehouse. They are separate products or capabilities. The complete local operational console, including less prominent CLI tools, is the first release scope.
