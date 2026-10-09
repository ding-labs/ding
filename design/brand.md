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

The green bell in `assets/mark.svg` is the compact mark and favicon. Keep clear
space around it and a readable Ding wordmark beside it in navigation. Social
artwork lives in `assets/social.svg`; the website build generates a PNG for cards.
All source SVG assets are covered by the repository's Apache-2.0 license.

Use semantic colors with text or icons, never color alone. Navigation selection
uses a neutral surface. Green is not a generic claim that a watch is healthy.
Use restrained motion, visible keyboard focus, and reduced-motion support.

Website builds copy tokens/assets. Docs builds stage them and map tokens to
Material theme variables. The console can replace its theme import with `../../../design/console.css`
after this branch is integrated. That adapter preserves its existing variable
names and the U03 theme values while consuming the shared foundation. Keep app components local;
do not copy the public page layout into operational screens.

Public screenshots must name their version and use deterministic data. The
current website illustration is explicitly a design preview, not a released
console screenshot. Update `content/product.json` only after verifying the
runtime artifacts and console minimum version.
