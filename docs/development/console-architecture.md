# Console implementation decisions

Evidence-first is the chosen layout. This document freezes the U01 decisions for implementation; [the product plan](console-plan.md) remains the complete scope.

## Repository and build

Use `web/console` for the static React/TypeScript app and `internal/webui` for Go embedding. A `console` Go build tag embeds the production build; ordinary Go development and tests remain headless. Release/container builds explicitly enable the tag after a locked frontend install/build. Missing assets fail a console build instead of producing a broken release. `ding ui --no-open` prints a one-use launch URL; `ding ui` also opens the browser.

Keep `workers/website` and the independent public-site workflows in place. The merged website/docs work builds versioned documentation into `workers/docs/site` and selects one production publisher through `DOCS_PUBLISHER`. GitHub Pages remains the default until an explicit Cloudflare cutover; console integration does not change that setting or deploy a site. Use the staged documentation build and generated CLI navigation described in the [publishing guide](../contribute/index.md).

## Browser boundary

The daemon serves `/ui/` and `/v1/`. The CLI obtains a 60-second single-use handoff using its existing admin bearer credential. The browser exchanges the URL-fragment handoff for an HttpOnly, SameSite=Strict session; the fragment is cleared before any request. Sessions last at most eight hours, are memory-only and bounded, and disappear on restart. Responses expose a per-session CSRF value, never the admin or ingest token.

Cookie requests require an exact configured Host; mutations additionally require the configured Origin and CSRF header. The exchange also requires Origin. Ingest routes never accept session cookies. Bearer API behavior stays compatible. Remote browser access requires an explicitly configured HTTPS UI origin; forwarded headers are not trusted. Browser access is optional for existing remote API deployments. Logout revokes the session. Cookie names include an origin hash so two daemon ports on the same host remain independently usable.

Static assets get correct types, content hashes, CSP and framing protection. Only recognized console navigation paths receive the SPA fallback; missing assets and `/v1` errors remain errors. Tool uploads, heavy proof inspection, diagnostics and backup are bounded independently from ordinary list polling.

## Contracts and evidence

Existing API shapes stay stable. Add `/v1/info`, `/v1/status`, `/v1/console/watches`, `/v1/console/events`, `/v1/console/deliveries`, `/v1/console/destinations`, and bounded retained-observation access. Lists use 50 rows by default and a hard maximum of 100; cursors include store identity, filter scope, snapshot boundary and order. Retention invalidation returns an explicit gap. Counts are complete server counts, not counts of the displayed page. Definitions/evidence are lazy detail requests.

Add bounded `/v1/tools/compile`, `/v1/tools/test`, `/v1/tools/replay`, `/v1/tools/migrate`, `/v1/console/doctor`, and `/v1/console/backup` creation/download. Reuse Go compiler/replay/converter/store functions. Generate frontend contracts from Go types and check drift. Whole-bundle apply review includes destination changes and atomic watch/destination revision preconditions. No JavaScript evaluator and no arbitrary shell endpoint.

## Screen/state contract

| Journey | Presentation and required data | Unhappy path |
| --- | --- | --- |
| Find attention | Watch name/ID; lifecycle; complete entity/incident/source counts; delivery failures; latest input | Waiting for first input differs from no incident; disconnected data includes its age |
| Explain firing | Event-time definition; checkpoint prior state; latest accepted observation; condition; exact event; related deliveries | Checkpoint-only prior state is labeled; lifecycle events may have no replay proof; retained history can have gaps |
| Diagnose delivery | Recorded destination revision, attempt outcome/detail, queue status, next retry | Source error and receiver error stay distinct; terminal-only manual retry; uncertain receipt can duplicate |
| Edit/apply | Authoritative YAML; deterministic explanation; validation diagnostics; fixture output; review diff; state reset/preservation | Draft survives errors/reauthentication; editing invalidates review; conflicting revision requires a fresh comparison |
| Pause/resume/delete | Current revision and committed lifecycle result | Pause retains incident and committed deliveries; delete has an explicit cancel-pending choice; reconcile timed-out writes |
| Operate | Explicit Doctor run; credential names/presence; limits; export; verified backup; migration and replay reports | No missing credential values are requested from the browser; unsafe server paths cannot become downloads |

Routes: `/ui/watches`, `/ui/watches/:id`, `/ui/events`, `/ui/events/:id`, `/ui/deliveries`, `/ui/deliveries/:id`, `/ui/workbench`, `/ui/system`. Filters and tabs live in the URL; payloads, drafts and secrets do not. Inspector links open as full pages and browser Back restores context. Theme, density, time display and instance-scoped saved filters are the only default persistent browser preferences.

Use the selected evidence-first concept's subdued rail, readable headings, four-stage evidence strip, retained chronology, and contextual evidence panel. Design fixtures cover empty, no input, active incident, held incident/unknown, recovery, source failure, queued/failed delivery, paused/deleted watch, stale session, conflict, and expired cursor. Actual data availability decides which evidence is rendered; never synthesize an observation from a counter.

## Phase gates

U01 is complete when the selected design, screen/error contracts, repository decisions and command inventory are recorded and the CLI inventory test passes. U02 must pass auth/static/deep-link tests and both build modes before U03 reads. U03 read/pagination tests precede U04 writes. Subsequent gates and browser/runtime qualification remain as specified in the product plan. Record actual checks in [console progress](console-progress.md).

## Read consistency

Watch summaries are live, timestamped snapshots with lexical-ID pagination. Aggregate counts cover the complete query, not the loaded page. A watch changing state may enter or leave a filtered view between pages; refreshing starts a current view. Event history fixes an upper sequence boundary when opened, pages newest first, and exposes a forward cursor for explicit live following. Cursors are bound to the store, query and page limit. Any intervening retention-floor change invalidates an event-history cursor with `410`; the reader must explicitly load available history. Summary responses exclude raw observation fields, intent payloads and replay checkpoints. Those are fetched only for the selected detail and rendered in bounded, expandable previews.

## Editing and resource decisions

The authoritative editor is an accessible plain textarea. Users may enable a lazily loaded CodeMirror 6 editor (exact component versions in `web/console/package-lock.json`); it shares the draft and supports undo, YAML highlighting, line numbers and keyboard navigation. The plain editor remains available for assistive technology and diagnostic line selection. Syntax colors use the same tested light/dark tokens. This keeps the initial bundle below 250 KiB gzip and avoids making a code editor a prerequisite for creating a watch.

Visible watch/list polling runs every five seconds, live event follow every three seconds, and instance summaries every fifteen seconds. Background polling is disabled. Proof verification occurs only for selected evidence, after immutable evidence has been read and the database transaction released; the handler admits at most two concurrent proof inspections. Doctor and backup each have their own concurrency limits. SQLite remains the sole authoritative evaluator/store.

Mutations are never automatically retried. A lost or unreadable response requires inspecting committed state or comparing current definitions before a further submission. Drafts survive the reconciliation. Backup artifacts are bound to authenticated session identity and are not transferable between browser sessions.
