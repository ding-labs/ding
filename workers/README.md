# Public website and documentation publishing

The root Website and Docs workflows build and verify both surfaces, then deploy
only their own artifact. Production deployment is restricted to main. PR checks
never receive deployment credentials. Artifacts include source commit metadata,
all documentation channels, browser screenshots, and a verification report.

## Local verification

From the repository root, install the locked website/docs npm dependencies and
Python docs requirements. Run:

```sh
npm run build --prefix workers/website
npm test --prefix workers/website
.venv/bin/python scripts/web/build_docs.py
.venv/bin/python scripts/web/check_links.py
.venv/bin/python scripts/web/smoke_quickstart.py
npm run check --prefix workers/docs
npm run check:browser --prefix workers/website
```

The browser check starts temporary local Workers for both sites and stops them
when finished. Install its Chromium once with `npx playwright install chromium`
from `workers/website`. Optional `SITE_URL` / `DOCS_URL` use existing local servers;
`PLAYWRIGHT_CHROMIUM_EXECUTABLE` selects an installed compatible browser.

`npm run dev --prefix workers/website -- --port 4317` keeps a local website preview
open. Docs: run `npx wrangler dev --port 4318` inside `workers/docs` after the build.
No public page connects to a local daemon or asks for credentials.

## Website publication

The website Worker owns `ding.ing` and the canonical `www.ding.ing` redirect.
`npm run deploy:preview --prefix workers/website` publishes a separate noindex
Worker with no production routes. The production workflow checks that the advertised docs origin and runtime are
available, then publishes the exact checked artifact. Publish docs before the
first website cutover; rerun Website if it correctly waits on that dependency. Manual local production publication uses
`npm run deploy --prefix workers/website` after the full checks above.

Save the current deployment/version identifier before publishing. `wrangler
versions list` and `wrangler deployments list` expose it. Roll back using
`wrangler rollback <verified-version-id>` in the component directory and check
its home, a deep link, assets, and an intentional missing URL. Keep the previous
build artifact; a content rollback must preserve needed documentation snapshots.

## Documentation cutover

`DOCS_PUBLISHER` is the repository setting for the single production publisher.
Unset or `github-pages` preserves the existing GitHub Pages path. `cloudflare`
selects the prepared `ding-docs` Worker. An invalid value fails publication.
Changing this setting is a hosting cutover, separate from merging content changes.

1. Record the GitHub Pages origin/custom-domain settings, Cloudflare DNS records,
   last deployment identifiers, and the last complete artifact.
2. Build all channels and publish `npm run deploy:preview --prefix workers/docs`.
   Check current docs, `/preview/`, `/legacy/v0.14.0/`, search, generated CLI pages,
   old entry URLs, slash redirects, missing assets, and HTTPS on the preview origin.
3. Confirm the production custom domain is ready and credentials can deploy to
   the configured Cloudflare account. The draft account ID is retained from the
   original configuration; verify ownership before changing domain routing.
4. Set `DOCS_PUBLISHER=cloudflare`, dispatch Docs with target `production`, and
   transfer the domain to the new Worker. Avoid concurrent publications during
   this short handover. Check representative production routes and search.
5. Retain the old Pages artifact and origin for rollback. If validation fails,
   restore the saved domain configuration and select `github-pages`, or restore
   the previous Cloudflare version if only the Worker deployment changed.

Only one production publishing step runs. The old nested website workflow has
been retired. A domain lookup alone does not verify account/domain ownership.
The complete docs build regenerates retained archives from pinned source tags;
`--current-only` artifacts are rejected by publication checks.

## Release coordination

`content/product.json` pins the documented runtime and declares that the console
is still a design preview. Updating it to a released console deliberately fails
the website build until preview artwork and copy are replaced with tested release
content. Add the release tag to `content/docs-versions.json`, build the immutable
snapshot, verify installer/artifacts, and publish matching docs before enabling
new release links. Help links in the console should use the compatible snapshot.

Future version snapshots must retain their own release metadata. When adding one,
verify the channel banner, installation ref, and snippets against that tag. Do not
silently relabel a legacy archive or publish development-only commands as stable.
