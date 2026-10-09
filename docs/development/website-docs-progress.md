# Website and documentation implementation progress

Implementation follows [the migration plan](website-docs-plan.md). The work lives
on `codex/website-docs`, based on `1275a45`. The separate Console branch continues
to own runtime/UI behavior. The original checkout's untracked hosting drafts and
plan were preserved and incorporated into this isolated change.

| Phase | Status | Evidence or remaining gate |
| --- | --- | --- |
| W00 Inventory | Complete for repository and current origin | `content/url-inventory.json`; GitHub Pages reports `https://ding-labs.github.io/ding/`, no custom domain. `docs.ding.ing` did not resolve. Restored Cloudflare access confirms the configured account and existing website Worker; production docs routing still needs verification at cutover. |
| W01 Shared brand | Implemented for public surfaces | Shared light/dark tokens, bell mark, social art, brand guide, availability metadata. Values follow the Console's U03 theme. `design/console.css` is ready for integration into the separate UI branch. |
| W02 Accuracy corrections | Implemented | Watch-first website and docs; source preview versus legacy release; correct license, sources, storage, delivery guarantees, and explicitly labeled Console preview. |
| W03 Publishing | Implemented with live review deployments | Independent root workflows, pinned dependencies, clean staged builds, preserved Pages publisher, Cloudflare previews, complete-channel guard and publication runbook. Production cutover awaits repository deployment credentials. |
| W04 User documentation | Implemented and verified | First watch, concepts, source/condition guides, delivery/evidence, operations/backup, full CLI generation, manifest schema, current API, frozen legacy archive. |
| W05 Website redesign | Implemented and verified | Home, Console preview, Examples, real 404s, theme preference, fixture illustration, metadata and legacy redirects. Browser screenshots and report are generated under `artifacts/public-surfaces/`. |
| W06 Console release help | Gated by the UI release | Availability page and working CLI alternatives are present. Real screenshots and procedural UI guides await completed U04–U07 behavior and a selected executable version. No mockup is presented as released software. |
| W07 Launch | Pending external prerequisites | Configure repository deployment credentials, verify production docs routing, publish docs, retire the old `ding-web` publisher, then publish website. Keep previous deployment artifacts for rollback. Console launch additionally requires its release gates. |

## Verification

- Strict MkDocs builds for current, development preview, and v0.14.0 legacy docs.
- More than 12,900 internal link and fragment checks across both origins.
- Real temporary-daemon quickstart: validate all six manifests, unchanged dry-run,
  apply, duplicate receipt, firing, recovery, evidence replay, restart persistence,
  Doctor, and pause. No existing daemon/state directory or qualification soak used.
- Node tests cover canonical-host and legacy redirects, missing assets, release
  metadata guardrails, publication content, and preview indexing controls.
- Chromium checks cover seven pages, four widths (375/768/1024/1440), both themes,
  serious/critical accessibility findings, fixture controls, keyboard theme control,
  theme persistence, 200% zoom-equivalent reflow, no-JavaScript content, routing,
  and documentation search. Screenshots are reviewed as well as audited.
- Workflow validation and both Cloudflare Worker packaging dry runs pass.
- Both npm dependency audits report zero known vulnerabilities after updating and
  pinning the deployment toolchain.
- The website uses approximately 10 KiB initial transfer on the recorded cold
  mobile profile, with zero measured layout shift. Build/browser checks enforce
  the 50 KiB gzip JavaScript and 1 MiB initial-transfer budgets. Docs performance
  is recorded separately; cross-origin responses without timing permission are
  identified as unmeasured. Review-deployment measurements are in the browser report.

## Deployment prerequisites

Cloudflare login was restored and the configured account was verified. Review
deployments are available at [the website preview](https://ding-website-preview.zuchka.workers.dev)
and [documentation preview](https://ding-docs-preview.zuchka.workers.dev). Website
preview links and legacy redirects use the documentation preview; production
builds retain the canonical documentation origin. Both previews exclude indexing.

The GitHub repository currently has no Cloudflare deployment token/account secrets.
No production deployment or domain change was performed. The Docs workflow keeps
GitHub Pages as the selected publisher until `DOCS_PUBLISHER=cloudflare` is set
for a deliberate, verified cutover. Website publication checks that the advertised
documentation origin and runtime metadata are already available.

The current production website version was recorded as
`e3e8bb1f-9adc-4f53-bec9-fa334501c36d`; record it again immediately before cutover
in case the old repository has published a newer version.

The separate `ding-labs/ding-web` repository still has an active deployment
workflow targeting the same website Worker and domains. It remains active until
the replacement is ready; the publishing runbook records its retirement and
rollback commands. Removing the nested workflow from this monorepo does not
disable that independent publisher.

Browser verification does not substitute for the plan's manual screen-reader task
session or a real production rollback rehearsal. These remain launch checks.
The branch adds no daemon authentication or evaluation behavior.
