import {
  readFileSync,
  writeFileSync,
  mkdirSync,
  rmSync,
  cpSync,
} from "node:fs";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { gzipSync } from "node:zlib";
import { loadProduct, escapeHtml as e } from "../../../scripts/web/product.mjs";
const root = new URL("../../../", import.meta.url);
const base = new URL("../", import.meta.url);
const out = new URL("dist/", base);
const product = loadProduct(root);
const preview = process.argv.includes("--preview");
const websiteUrl = product.websiteUrl;
const docsOrigin = preview ? product.previewDocsUrl : product.docsUrl;
const docsUrl = docsOrigin + "/";
const source = execFileSync("git", ["rev-parse", "HEAD"], {
  cwd: root,
  encoding: "utf8",
}).trim();
const availability =
  product.runtime.channel === "source-preview"
    ? "Watch runtime: source preview. Published v0.14.0 is legacy."
    : `Watch runtime: ${product.runtime.version}.`;
const cta =
  product.runtime.channel === "source-preview"
    ? "Try the watch preview"
    : "Get started";
const pages = [
  ["index.html", "", "Know what happened. See why.", product.description],
  [
    "console/index.html",
    "console/",
    "Ding Console",
    "An evidence-first interface for your Ding daemon. Inspect watch, event, and delivery evidence.",
  ],
  [
    "examples/index.html",
    "examples/",
    "Persistent watch examples",
    "Try API health, missing heartbeats, local commands, and value changes with versioned Ding watch manifests.",
  ],
  [
    "404.html",
    "404.html",
    "Page not found",
    "Find current and legacy Ding documentation.",
  ],
];
const evidence = `<div class="evidence-window" aria-label="Illustrative watch evidence, not a live daemon">
  <div class="window-top"><span class="mini-brand"><img src="/assets/mark.svg" width="22" height="22" alt=""> Ding Console</span><span class="sample-label">SAMPLE DATA</span></div>
  <div class="window-body"><div class="breadcrumb">Watches <span aria-hidden="true">/</span> api-health</div>
  <div class="watch-heading"><div><span class="eyebrow">Selected event</span><h2>API health</h2></div><span class="status incident" data-demo-badge>Incident opened</span></div>
  <p class="watch-explanation" data-demo-title>Three checks.<br>Three server errors.</p>
  <p class="muted" data-demo-description>The third matching response opened this incident.</p>
  <div class="evidence-strip"><div><span>01 · Observed</span><strong data-demo-observed>503</strong><small>HTTP status</small></div><div><span>02 · Evaluated</span><strong data-demo-count>3 of 3</strong><small data-demo-rule>matching checks</small></div><div><span>03 · Recorded</span><strong data-demo-event>Firing</strong><small>event + evidence</small></div><div><span>04 · Delivered</span><strong>Not shown</strong><small>fixture sends nothing</small></div></div>
  <div class="history-title"><h3>Retained observations</h3><span>Jan 1 · UTC</span></div>
  <div class="observation"><span>00:00:00</span><span>500</span><span>Matching</span></div><div class="observation"><span>00:00:05</span><span>502</span><span>Matching</span></div><div class="observation selected"><span data-demo-time>00:00:10</span><span data-demo-code>503</span><span data-demo-outcome>Incident opened</span></div>
  <div class="demo-controls" hidden><span>Explore the fixture</span><button type="button" data-demo="firing" aria-pressed="true">Firing</button><button type="button" data-demo="recovered" aria-pressed="false">Recovery</button></div></div></div>`;
const fixture = readFileSync(
  new URL("testdata/watches/api-health.jsonl", root),
  "utf8",
)
  .trim()
  .split("\n")
  .map(JSON.parse);
const manifest = readFileSync(
  new URL("examples/watches/api-health.yaml", root),
  "utf8",
).split("---\n")[1];
const values = {
  docsUrl,
  installUrl: docsOrigin + product.installPath,
  quickstartUrl: docsOrigin + product.quickstartPath,
  cta,
  availability,
  evidence,
  manifest: e(manifest),
  fixture: fixture
    .map((r) => `<span>${e(r.fields["http.status"])}</span>`)
    .join('<span aria-hidden="true">→</span>'),
};
rmSync(out, { recursive: true, force: true });
mkdirSync(new URL("assets/", out), { recursive: true });
cpSync(new URL("design/assets/", root), new URL("assets/", out), {
  recursive: true,
});
cpSync(new URL("design/tokens.css", root), new URL("assets/tokens.css", out));
for (const [name, asset] of [
  ["ding-logo.svg", "bell-dark.svg"],
  ["ding-logo-light.svg", "bell-light.svg"],
]) {
  cpSync(new URL(`design/assets/${asset}`, root), new URL(name, out));
}
for (const name of ["style.css", "site.js"])
  cpSync(new URL(`site/${name}`, base), new URL(`assets/${name}`, out));
const scriptBytes = gzipSync(
  readFileSync(new URL("assets/site.js", out)),
).length;
if (scriptBytes > 50 * 1024)
  throw new Error(
    `Website JavaScript exceeds the 50 KiB gzip budget: ${scriptBytes} bytes`,
  );
for (const [filename, path, title, description] of pages) {
  let content = readFileSync(new URL(`site/${filename}`, base), "utf8").replace(
    /\{\{(\w+)\}\}/g,
    (_, key) => {
      if (!(key in values)) throw new Error(`Unknown template value ${key}`);
      return values[key];
    },
  );
  const nav = [
    ["/", "Product"],
    ["/console/", "Console"],
    ["/examples/", "Examples"],
    [docsUrl, "Docs"],
    [product.repositoryUrl, "GitHub"],
  ]
    .map(
      ([href, label]) =>
        `<a href="${e(href)}"${href === "/" + path ? ' aria-current="page"' : ""}>${label}</a>`,
    )
    .join("");
  const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"><title>${e(title)} — Ding</title><meta name="description" content="${e(description)}"><link rel="canonical" href="${websiteUrl}/${path}"><meta property="og:title" content="${e(title)} — Ding"><meta property="og:description" content="${e(description)}"><meta property="og:type" content="website"><meta property="og:url" content="${websiteUrl}/${path}"><meta property="og:image" content="${websiteUrl}/assets/social.png"><meta property="og:image:alt" content="Ding. Know what happened. See why."><meta name="twitter:card" content="summary_large_image">${preview || path === "404.html" || path === "console/" ? '<meta name="robots" content="noindex,follow">' : ""}<link rel="icon" href="/assets/mark.svg" type="image/svg+xml"><link rel="stylesheet" href="/assets/tokens.css"><link rel="stylesheet" href="/assets/style.css"><script src="/assets/site.js" defer></script></head><body><a class="skip-link" href="#main">Skip to content</a><header class="site-header"><a class="wordmark" href="/" aria-label="Ding home"><img src="/assets/mark.svg" width="34" height="34" alt=""><span>Ding<span class="wordmark-dot">.</span></span></a><nav aria-label="Main navigation">${nav}</nav><button class="theme-toggle" type="button" hidden aria-label="Change color theme">Theme: system</button></header><main id="main">${content}</main><footer><a class="wordmark" href="/"><img src="/assets/mark.svg" width="28" height="28" alt="">Ding.</a><p>Persistent watches. Durable alerts.</p><div><a href="${docsUrl}">Documentation</a><a href="${product.repositoryUrl}">GitHub</a><a href="${product.repositoryUrl}/blob/main/LICENSE">Apache-2.0</a></div></footer></body></html>`;
  const dest = new URL(filename, out);
  mkdirSync(new URL("./", dest), { recursive: true });
  writeFileSync(dest, html);
}
writeFileSync(
  new URL("sitemap.xml", out),
  `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${pages
    .filter((p) => !["404.html", "console/"].includes(p[1]))
    .map((p) => `<url><loc>${websiteUrl}/${p[1]}</loc></url>`)
    .join("")}</urlset>`,
);
writeFileSync(
  new URL("robots.txt", out),
  preview
    ? "User-agent: *\nDisallow: /\n"
    : `User-agent: *\nAllow: /\nDisallow: /console/\nSitemap: ${websiteUrl}/sitemap.xml\n`,
);
writeFileSync(
  new URL("_headers", out),
  `/*\n  X-Content-Type-Options: nosniff\n  Referrer-Policy: strict-origin-when-cross-origin\n  Content-Security-Policy: default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'\n/assets/*\n  Cache-Control: public, max-age=3600\n`,
);
writeFileSync(
  new URL("build-info.json", out),
  JSON.stringify(
    { source, runtime: product.runtime, console: product.console, preview },
    null,
    2,
  ) + "\n",
);
console.log(`Built website → ${fileURLToPath(out)}`);
