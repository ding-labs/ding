# Ding brand and interface foundation

Use Ding in prose and Ding Console for the operational interface. Lead with the
chain observation → decision → event → delivery. The interface labels are
Observed, Evaluated, Recorded, Delivered. State exactly what happened and what
the reader can do next. Use watch, source, condition, event, destination, and
delivery consistently; a running watch can have an open incident or failed delivery.

`tokens.css` is the shared source for warm light and graphite dark surfaces,
green actions, text, spacing, and named status colors. It follows the selected
evidence-first console plan. Use system sans-serif and system monospace; no
external font request is required. Keep body text at least 14px in product views
and 16px in long-form public content. Use 4/8/12/16/24/32px spacing increments.

The fire-alarm bell in `assets/mark.svg` is restored from the original website,
`workers/website/site/ding-logo-light.svg`, introduced in website commit `c449b07`.
Its paths and colors are unchanged. `bell-light.svg` and `bell-dark.svg` preserve
the original two treatments. The website, docs, and console use this same mark
and favicon. Keep the light mark on a small ivory backing in dark navigation.
Social artwork lives in `assets/social.svg` with its matching rendered PNG.
All source SVG assets are covered by the repository's Apache-2.0 license.

Use semantic colors with text or icons, never color alone. Navigation selection
uses a neutral surface. Green is not a generic claim that a watch is healthy.
Use restrained motion, visible keyboard focus, and reduced-motion support.

Website builds copy tokens/assets. Docs builds stage them and map tokens to
Material theme variables. The console imports `../../../design/console.css` from its theme entry point.
That adapter preserves its existing variable names while consuming the shared
foundation. Keep app components local;
do not copy the public page layout into operational screens.

Public screenshots must name their version and use deterministic data. The
current website illustration is explicitly a design preview, not a released
console screenshot. Update `content/product.json` only after verifying the
runtime artifacts and console minimum version.


## Original-green design draft — October 9, 2026

Review `draft/index.html` for light and dark captures of all three working surfaces.
These captures use isolated console fixtures; they are not live operational data.
The draft is based on main `dd4715e` and is not a production deployment.

| Role | Color | Use |
| --- | --- | --- |
| Original Ding green | `#7EE787` | Unchanged bell, primary action backgrounds, hero highlight |
| Warm ivory | `#F7F6F2` | Light canvas, light logo backing |
| Graphite | `#222B25` | Light text; nearby `#171C19` is the dark canvas |
| Soft violet | `#ECE5F3` | Marketing product surround and informational accents |
| Violet ink | `#68538B` / `#C2ADDF` | Readable light/dark informational and unknown labels |
| Amber | `#80530F` / `#EBBC74` | Attention and pending states, always labeled |
| Pine | `#246239` | Readable links on light surfaces |
| Coral | `#A33C33` / `#F0A398` | Incidents and failures, always labeled |

Green buttons use dark `#17251C` text (10.4:1 contrast). Do not use the original
bright green as small text on ivory. Dark-theme links use the original green;
light-theme links use pine. Navigation remains neutral to avoid implying success.

The website uses a green headline highlight and a soft violet product surround.
The console keeps the same palette quieter: neutral working surfaces, green
primary actions, and separate condition, source, and delivery states. Docs use
pine links, a green heading rule, quiet code backgrounds, and violet notes.

For local review, build the website and docs with their existing scripts and build
`web/console`. Open `design/draft/index.html` directly, or serve `design/` with a
local static server. The review sheet needs no framework, account, or daemon.
