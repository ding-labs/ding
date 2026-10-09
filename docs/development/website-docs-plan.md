# Ding website and documentation migration plan

Status: implementation underway, October 9, 2026. See
[implementation progress](website-docs-progress.md) for completed checks and launch gates.

Update `ding.ing` and `docs.ding.ing` around persistent watches and the selected
evidence-first Ding Console. Keep the website, documentation, and application
independently deployable while sharing product language, brand assets, design
tokens, and verified examples. Correct misleading public content first; launch
console documentation and imagery as the corresponding application features ship.

The companion UI plan is `docs/development/console-plan.md`, revision `956178d`
on `codex/console-design-plan`, now being implemented on `codex/ding-console`.
Its U01–U07 phases own console behavior, APIs, authentication, and embedding.
This plan owns public communication, documentation, and their publishing paths.

## Starting point and corrections

The primary checkout was reviewed at `9326ccc`. The existing untracked
`.github/workflows/website.yml` and `workers/docs/` are hosting work in progress
and must be reconciled before workflow changes. These observations describe the
reviewed state, not a permanent release status.

| Surface | Current state | Required change |
| --- | --- | --- |
| Website | `workers/website/site/index.html` markets ephemeral jobs, `ding run`, legacy rules, no database, and MIT licensing. It includes old integrations and performance figures. | Rewrite for the watch runtime, local SQLite persistence, Apache-2.0, and supported sources/destinations. Remove unsupported claims and obsolete benchmarks. |
| Documentation | MkDocs Material in `docs/` already introduces watches, but onboarding is thin and some development milestones describe superseded behavior. | Organize around user journeys; add concepts, operational guides, console help, and clear release boundaries. Mark historical milestones explicitly. |
| Brand | Website and docs use dark terminal styling and monospace body text; logos differ. | Follow the selected console direction: warm light surfaces, graphite dark surfaces, restrained green, readable sans-serif text, and consistent state language. |
| Publishing | Website has a nested workflow from its former repository. The root website workflow is untracked. Docs still run `mkdocs gh-deploy --force`, while an untracked Cloudflare docs Worker expects `workers/docs/site/`. | Establish one root workflow per publisher and a reproducible build for each. Complete the docs hosting transition before retiring the old publisher. |
| Release guidance | README/install docs identify v0.14.0 as legacy and the watch runtime as a source preview. Console work is underway. | Keep preview, released runtime, and console availability explicit in every install path and product claim. |

The [public website](https://ding.ing) also served the legacy positioning during
review. Production documentation could not be verified through the browsing
tool; validate its current origin, domain routing, and URLs during W00.

The root `.gitignore` uses an unanchored `site/` rule. Existing tracked website
files survive it, but new assets under that directory can be silently ignored.
Narrow generated-output ignores and verify source assets from a clean checkout.

## Product story and brand

Use **Ding** in prose and **Ding Console** for the application interface. Retain
the recognizable green bell and refine its wordmark, small-size treatment, and
light/dark variants as one asset family.

Proposed website headline: **Know what happened. See why.**

Supporting description: **Persistent watches and durable alerts for developers
and agents. Watch an API, run a local check, or receive events from your software.
Ding keeps the condition state, records the evidence, and retries notifications.**

Explain the product through **observation → decision → event → delivery**. The
console's corresponding labels are **Observed / Evaluated / Recorded / Delivered**.
Use these labels in screenshots, diagrams, tutorials, and interface explanations.
Describe retained evidence accurately, including unavailable history and
checkpoint-only evidence. A recorded event does not establish delivery success.

| Brand rule | Website | Documentation | Console alignment |
| --- | --- | --- | --- |
| Typography | Spacious headlines and readable sans-serif copy | Readable article width; monospace for code and exact values | Same font family and scale foundation; denser operational layouts |
| Color | Green for identity and primary actions | Green for links/actions where contrast passes | Brand and semantic status tokens remain distinct |
| Surfaces | Warm near-white and graphite themes | Same themes with clear code, table, and callout contrast | System theme plus an explicit preference on each origin |
| Imagery | Real product evidence and a simple architecture diagram | Annotated task screenshots with text equivalents | Capture deterministic fixtures from the implemented interface |
| Voice | Concrete outcomes and supported examples | Explain the action, expected result, and recovery path | Same names for watches, events, sources, destinations, and states |

Follow the UI plan's spacing, typography, and motion decisions. W01 records final
values rather than establishing a competing palette. Test green variants on both
backgrounds; the existing `#7ee787` is a brand reference, not an automatic choice
for light-theme text. Status always has a text label or symbol. Avoid terminal
decoration across whole pages, looping animations, and charts without retained data.

Keep these promises consistent:

- The daemon runs continuously and evaluates declared conditions without a model call.
- Developers, agents, CLI users, and console users operate the same watch definitions.
- Sources are HTTP, explicit local commands, and authenticated JSON push. Delivery
  supports console output, webhooks, Slack, and Discord according to the release.
- Persistent state and a durable outbox survive restart. Remote delivery is at
  least once; explain deduplication and bounded retention in the docs.
- The core is self-hosted and Apache-2.0. A hosted service, account system, built-in
  natural-language authoring, and bundled consumer data feeds are outside this launch.

## Repository and deployment boundaries

Retain the console plan's monorepo layout. Continue using static HTML for the
website and MkDocs Material for documentation. Neither surface needs the console's
React application or an additional framework migration to adopt the new brand.

| Responsibility | Source | Build and delivery |
| --- | --- | --- |
| Runtime and CLI | `cmd/`, `internal/` | Existing Go releases and containers |
| Ding Console | `web/console/`, `internal/webui/` | Static application embedded in its matching Go executable |
| Public website | `workers/website/site/`, `workers/website/src/` | Small static build into `workers/website/dist/`; independent Cloudflare deployment |
| Documentation | `docs/`, `mkdocs.yml`, `cmd/docgen/` | Generate references and run MkDocs into `workers/docs/site/`; independent Cloudflare deployment |
| Shared design | Proposed `design/brand.md`, `design/tokens.css`, `design/assets/` | Consume/copy at build time; no shared runtime service |
| Product availability | Proposed `content/product.json` | Small validated metadata file consumed by website/docs builds |
| Install endpoint | `workers/install/`, `scripts/install.sh` | Existing independent deployment, coordinated with verified release artifacts |

Share logo SVGs, fonts if selected, CSS custom properties, and editorial guidance.
Self-host appropriately licensed fonts with system fallbacks on all three surfaces.
Keep public navigation and article components local to their surface. Extract a
shared component library only after actual reuse warrants it. The docs theme maps
shared tokens into Material variables; the console imports the same foundation.
Generated asset copies are never separately edited.

`content/product.json` should record product name, license, release channel,
verified version/source ref, canonical docs/install links, and console availability
with its minimum version. It describes availability; Go contracts remain the
source of runtime semantics. Website/docs builds reject contradictory combinations
such as a console CTA pointing at a release that lacks the console.

The website stays useful without JavaScript. Add a small build entry point under
`workers/website/scripts/` to assemble metadata, shared assets, and authored HTML;
move inline styles into maintainable stylesheets. Update the Worker asset path to
the generated directory. Pin the tooling and document clean build commands.

Keep docs Markdown as the authoring source. Stage shared assets, generated CLI
reference, and later API reference during the docs build. Preserve CLI generation
from `cmd/docgen`. Use the U01/U02 API schema for API reference once it exists;
the watch manifest JSON Schema is not a specification of control API endpoints.
Until then, maintain the existing API reference against implemented handlers.

`ding.ing` and `docs.ding.ing` are public content origins. The console remains
served by the user's daemon under `/ui/`. Public pages must not request daemon
credentials or automatically probe localhost. An “Open console” instruction
teaches the released `ding ui` flow when available; it does not imply a hosted app.

## Website content and navigation

Start with a focused home page plus a Console overview and Examples page.
Link documentation for procedural detail and release notes. Public navigation is
**Product**, **Console**, **Examples**, **Docs**, and **GitHub**, with **Get started**
as the primary action. During the source preview, label the action **Try the watch
preview** and send it to the matching installation instructions.

| Home page section | Content and purpose |
| --- | --- |
| Hero | Product promise, explicit availability label, one primary CTA, and a link to the quickstart |
| Product view | Evidence-first watch detail showing a recorded event and delivery state; use a labeled design preview until a real capture exists |
| How Ding works | HTTP/command/push → daemon and SQLite state → conditions/events → durable delivery; CLI, agent tools, and Console connect to the same daemon |
| Three supported uses | API failure and recovery, a missing heartbeat, and a changed value or new provider event; each links to a runnable guide |
| Understand an alert | A short evidence strip showing the observed value, condition, recorded event, and delivery result |
| Work your way | Explain the CLI, Console, and agent-assisted authoring paths without suggesting an embedded language model |
| Operate it yourself | Persistence, retained evidence, backups, and delivery retries, linked to operational docs |
| Start and migrate | One version-correct installation path, release notes, and a clearly visible legacy migration link |

Use the existing API-health fixture for the central evidence story: three real
5xx responses, an incident, and recovery after two nonmatching observations.
Keep the deterministic push-latency example as the first-install tutorial.
Reuse repository manifests and fixtures instead of maintaining decorative YAML.

Remove the old `rules:` walkthrough, `ding run`/`serve` instructions, end-of-run
claims, obsolete integrations, old binary-size/latency/throughput figures, and
legacy recipe links from the current-product path. Historical commands can remain
in explicitly versioned legacy material. Publish new performance claims only with
the matching artifact, measured workload, environment, and qualification evidence.

Include page-specific titles/descriptions, canonical URLs, a sitemap, favicon,
social preview asset, useful 404 page, and accessible links. Preserve meaningful
existing fragments such as `#install`. Index production content; exclude deployment
previews, design previews, and development documentation from search indexing.

## Documentation structure and journeys

Organize navigation by what the reader needs to accomplish. Keep stable existing
entry URLs where useful and add focused pages beneath them.

| Section | Pages and outcomes |
| --- | --- |
| Start | Product overview, installation and release selection, first watch, open the console, where state lives |
| Concepts | Watch and destination; observation/condition/event/delivery; lifecycle versus incident versus source health; revisions, evidence, retention and retry semantics |
| Guides | HTTP monitoring; push integration; command checks; missing data; changed values/provider events; Slack/Discord/webhooks; grouped watches; investigate an alert; resolve failed delivery |
| Console | Connect and authenticate; Watches; event/evidence inspection; Deliveries; Workbench validation/test/review/apply; System and backups; keyboard help and connection states |
| Operate | Persistent daemon setup, containers, secrets, remote TLS/proxy setup, resource limits, backup/restore, upgrade, diagnostics and troubleshooting |
| Reference | Watch manifest and JSON Schema; complete generated CLI; implemented HTTP API; error codes and state glossary |
| Migrate and releases | Legacy v0.14.0, supported conversion and gaps, watch release notes, compatibility and availability |
| Contribute | Monorepo build map, local development, generated artifacts, design rules, architecture contracts, tests and release qualification |

The first-watch tutorial must work without a paid account, third-party webhook,
or live external API: install/build the specified version, start a private state
directory, apply `ding.yaml.example`, push a high reading, inspect the firing
event, push a recovery reading with a new idempotency key, and inspect recovery.
Show expected output and a concise troubleshooting path. Offer equivalent console
steps only when those operations are present in the documented version.

Every procedural guide states prerequisites, version/channel, execution location,
commands or interface steps, expected result, and relevant failure recovery.
Use the same IDs and manifests in CLI and console examples. Show safe placeholders
for credentials; explain references and credential presence rather than displaying
real values. Explicitly distinguish a command on the reader's machine, the daemon
host, and a container.

Console help follows U03–U06 as capabilities land. Cover waiting for first input,
unknown condition with an open incident, source failure, delivery failure, paused
acquisition with pending delivery, expired history, lost browser connection, and
revision conflicts. These distinctions are essential to understanding the product.

Keep `install.md`, `configuration.md`, `api.md`, `examples.md`, and `legacy.md`
as useful entry pages during reorganization. Expand them or link to deeper guides
before moving any URL. Classify `docs/development/` pages as current contracts,
historical milestones, or contributor plans; remove stale present-tense claims
from current guidance and label preserved history. Keep planning/qualification
details outside the primary getting-started navigation.

## Release channels and URL migration

Public availability must follow released artifacts, not a merged UI design or a
screenshot. Use this publication model:

| Channel | Publication rule |
| --- | --- |
| Current docs at `/` | During watch preview, prominently identify the source ref and source-build requirement. After a watch release, document the current verified release. |
| Development docs at `/preview/` | Built from a recorded development ref; clearly labeled and excluded from indexing. Describe landed behavior. Keep unfinished UI drafts in review previews. |
| Legacy at `/legacy/v0.14.0/` | Preserve applicable legacy documentation from its source tag, with a version banner and migration link. |
| Future release snapshots at `/v/<tag>/` | Immutable docs tied to each published watch release; current docs link to the appropriate snapshot. |

Maintain one docs publishing job that assembles these channels into its deployment
artifact. Updating current docs must preserve retained snapshots and legacy pages.
The console's help links target its compatible docs version, with a useful fallback
for a preview build and an explanation when offline.

Create a checked-in URL inventory before reorganizing content. For each old URL,
record preserve, permanent redirect, versioned legacy destination, or genuine 404.
Account for `/install/`, `/configuration/`, `/api/`, `/examples/`, `/legacy/`,
generated `/cli/` pages, and website `/recipes/*` links. A retired recipe should
explain its legacy status rather than masquerade as a supported watch integration.
Preserve important anchors or supply compatibility anchors on the destination.

Retain `www.ding.ing` → `ding.ing`, including path and query. Verify direct deep
links, trailing-slash normalization, redirects, search assets, and missing files
on Cloudflare. Use explicit HTML routing behavior compatible with generated
directory indexes; Cloudflare documents those choices in its
[HTML handling reference](https://developers.cloudflare.com/workers/static-assets/routing/advanced/html-handling/).
Public content needs real 404 responses, not the console's SPA fallback.

## Implementation sequence

Ownership below describes responsibilities, not additional teams that must be
staffed. One contributor can fill several roles. Each phase ends with a reviewable
change and recorded acceptance results.

| Phase | Owner and dependencies | Deliverables | Exit gate |
| --- | --- | --- | --- |
| W00 Inventory and publishing agreement | Website/docs implementer; coordinate with U01 | Content/URL inventory, release-claim list, current origin verification, agreement on the existing untracked hosting work and single docs publisher | Every existing entry point and deployment has a disposition; current versus planned capabilities are explicit |
| W01 Shared brand and product contract | Design owner with U01; after W00 | Shared asset family, light/dark tokens, typography, voice/state glossary, product availability metadata, website/docs page samples | Home, docs article, and console watch detail read as one product; status semantics and contrast agree |
| W02 Public accuracy corrections | Website/docs implementer; after W00, can overlap W01 | Correct homepage positioning, license, preview/install guidance, unsupported claims, and broken legacy routes | A new visitor reaches a working version-correct first-watch path; no console feature is advertised as released prematurely |
| W03 Reproducible publishing | Build/release owner; after W00, incorporates W01 assets | Root website/docs checks and deployment workflows, clean builds, scoped dependencies, staging URLs, Cloudflare docs cutover and rollback procedure | Both surfaces build from a clean checkout and deploy independently; docs search/assets/deep links work; one publisher owns the production domain |
| W04 User documentation | Docs owner; after W01/W03; runtime contracts available now | New navigation, concepts, first watch, operational guides, reference generation, legacy archive and URL map | Fresh-state quickstart fires and recovers; examples validate; reference/navigation/link checks pass |
| W05 Website redesign | Website/design owner; after W01/W03, links to W04 | New home, Console overview, Examples, responsive themes, product illustrations, metadata and redirects | All CTAs resolve to the right channel; mobile/keyboard/theme checks pass; all product depictions are labeled accurately |
| W06 Console guides and real imagery | Docs/design owner; follows U03–U06 | Version-matched task guides, deterministic screenshots, contextual help links, parity walkthroughs | Each published UI procedure is demonstrated against the corresponding executable; no placeholder action is described as available |
| W07 Coordinated launch | Release owner; W04–W06 and U07 for console launch | Final claims audit, version snapshots, install/docs/website cutover, public route checks and rollback rehearsal | Published binary, installer, website, docs, and screenshots agree; production smoke checks pass and rollback artifacts are recorded |

W02 can ship before the redesign or console. W04 can ship the CLI/watch guidance
while W06 follows implementation. W05 may show a clearly labeled Console preview;
the full console launch waits for U07 and actual release artifacts. This sequence
lets public accuracy improve without coupling every website deployment to a binary
release.

## Build checks and deployment controls

| Changed inputs | Required checks and publication impact |
| --- | --- |
| Website source, Worker, or website tooling | Static build, links/redirects, representative browser checks; website deployment only |
| Docs, MkDocs config, docs tooling, or docs Worker | Reference generation, strict docs build, navigation/search/links, routing; docs deployment only |
| CLI/API/schema/example inputs | Runtime checks plus relevant generated-reference, example, and docs validation; console contract checks under the UI plan |
| Shared design or product metadata | Website, docs, and console consumer checks; publish eligible content surfaces and include console changes in its next tested binary |
| Install or release configuration | Artifact/channel consistency checks across installer, website, and docs; existing runtime release gates |

Include generator inputs such as `cmd/docgen/`, `internal/watchcli/`, API schema
and handlers, `schemas/`, examples, and dependency files in affected docs checks.
Include `design/` and `content/` in every consumer's change detection. Root workflows
are authoritative; retire the nested website workflow after its equivalent works.
Keep ordinary Go development usable without requiring website/docs build tools.

Run credential-free checks on pull requests. Use scoped deployment credentials
only for trusted publication jobs; serialize production deployment per surface.
Record source commit, release metadata, and deployment identifier. Use preview
URLs for review and maintain independent rollback artifacts for website and docs.
After stable launch, development changes publish to preview docs; updating current
docs requires selecting the documented release, even for an editorial correction.

Replace `mkdocs-material>=9.5` with a tested reproducible dependency set. Run
`go run ./cmd/docgen docs/cli`, then a strict MkDocs build against the staged source
tree. Configure missing-document, anchor, and navigation validation explicitly
using [MkDocs validation settings](https://www.mkdocs.org/user-guide/configuration/#validation);
strict mode alone is not a substitute for checking cross-site links and redirects.

Use an allowlisted historical-content audit for obsolete commands and claims so
legacy archives remain valid. Validate manifests with the documented binary and
replay fixtures without external I/O. Add one temporary-daemon smoke journey for
the first-watch tutorial; never run examples against a contributor's existing
state directory or the ongoing qualification soak.

For home, quickstart, reference, and console-guide pages, verify both themes at
375, 768, 1024, and 1440px, 200% zoom, keyboard navigation, visible focus, meaningful
headings, reduced motion, and text alternatives. Automated accessibility checks
must have no serious/critical findings; manual reading and keyboard checks remain
required. Keep code blocks usable on narrow screens and search results readable.

Provisional website budgets: no application framework bundle, at most 50 KiB gzip
of first-party JavaScript, and at most 1 MiB initial transfer on a mobile viewport.
Lazy-load lower-page imagery and reserve its dimensions. Record measured page
performance on a fixed mobile profile before release. Measure docs separately;
its search index has a different purpose and should be checked as content grows.

## Cutover and completion

First publish corrected content and review builds. For the docs hosting move,
capture the current domain configuration and old deployment, verify the new
Cloudflare artifact at a preview origin, then transfer the production domain.
Validate HTTPS, representative old URLs, redirects, assets, search, and 404s.
Disable the old GitHub Pages publisher only after the replacement is verified;
keep its last artifact and a documented way to restore the prior domain routing.

For console launch, publish and verify the runtime artifacts and compatible docs
before activating the website's console/install CTAs. Keep backward-compatible
URLs during the staggered deployments. Rehearse rollback of either content surface
without redeploying the daemon or removing versioned documentation.

Completion means a new visitor can understand Ding, choose the correct release,
create a watch, explain a real firing and recovery, and inspect delivery through
the documented CLI or released console. A legacy user can still find their
version and migration guidance. A contributor can build and deploy either public
surface independently, with shared brand and contract changes checked everywhere
they are consumed.

Resolve the active docs domain owner and cutover state during W00, and U01's final
token/font values during W01. Set the actual first console release tag at W07.
Public accuracy corrections and current-runtime documentation can proceed while
the console is being implemented.
