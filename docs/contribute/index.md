# Develop and publish the monorepo

Ding's Go runtime, embedded Console, public website, documentation, and installer
have separate builds. A website deployment does not release a daemon.

| Component | Source | Output |
| --- | --- | --- |
| Runtime and CLI | `cmd/`, `internal/` | Go executable and containers |
| Console in development | `web/console/`, `internal/webui/` on its implementation branch | Static assets embedded in the matching executable |
| Public website | `workers/website/site/` | `workers/website/dist/` → Cloudflare |
| Documentation | `docs/`, `cmd/docgen/`, `mkdocs.yml` | `workers/docs/site/` → independent publisher |
| Brand | `design/` | Build-time tokens and assets for each consumer |
| Product availability | `content/product.json` | Version-correct site and docs labels |

## Build locally

Use Node 24, Go from `go.mod`, and Python 3.11 or later. From the repository root:

```sh
npm ci --prefix workers/website
npm run build --prefix workers/website
npm test --prefix workers/website
python3 -m venv .venv
.venv/bin/python -m pip install -r docs/requirements.txt
.venv/bin/python scripts/web/build_docs.py
.venv/bin/python scripts/web/check_links.py
```

The docs build stages sources in a temporary directory, generates every CLI page,
copies canonical examples/schema/assets, and builds current, development-preview,
and archived documentation. It never writes generated reference into tracked
sources. `--current-only` is for local iteration and produces an artifact that
publication checks reject.

The website is static HTML with a small build-time template step. Its example
manifest and fixture are read from repository files. No frontend framework or
production Node process is required. To preview the generated website:

```sh
python3 -m http.server 4173 --directory workers/website/dist
```

## Keep changes aligned

Runtime contracts belong to Go and the versioned schemas. Change examples and docs
alongside behavior. Shared design tokens and availability metadata trigger checks
for both content surfaces. The Console consumes the same design foundation when
integrated; public code never imports runtime logic or requests local tokens.

`content/docs-versions.json` lists frozen source tags. Retain existing entries when
adding a release. `content/redirects.json` preserves legacy website recipe URLs.
The browser checker covers real links, fragment targets, small screens, both themes,
keyboard actions, and serious accessibility findings.

## Publish and roll back

Review the repository runbook in `workers/README.md`. Production jobs use scoped
credentials and publish independently. Documentation has a single selected
publisher; a repository setting controls its GitHub Pages to Cloudflare cutover.
Do not enable Cloudflare production publication until the domain routing and preview
checks pass. Keep the old artifact and domain settings for rollback.

A console release requires verified binaries, matching versioned docs, and real
screenshots. Changing an availability flag alone is intentionally insufficient to
replace preview content with a release claim.
