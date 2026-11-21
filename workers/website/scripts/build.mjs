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
import { createHash } from "node:crypto";
import { loadProduct, escapeHtml as e } from "../../../scripts/web/product.mjs";
const root = new URL("../../../", import.meta.url);
const base = new URL("../", import.meta.url);
const out = new URL("dist/", base);
const product = loadProduct(root);
const tellTail = JSON.parse(readFileSync(new URL("content/tell-tail.json", root), "utf8"));
const tellTailOut = new URL("workers/tell-tail/dist/", root);
const preview = process.argv.includes("--preview");
const tellTailHome = (preview ? tellTail.previewWebsiteUrl : tellTail.websiteUrl) + "/";
const websiteUrl = product.websiteUrl;
const cloudUrl = "https://cloud.ding.ing/";
const cloudAuthUrl = new URL("auth/login", cloudUrl).href;
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
const pages = [
  ["tell-tail/index.html", "tell-tail/", "Tell-Tail — Local AI observability for tool calls and MCP logs", "Inspect Codex tool results and matching local calls in AI Activity. Diagnose MCP failures and recovery, and stream application logs locally with Tell-Tail."],
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
const assetVersion = path => createHash("sha256").update(readFileSync(path)).digest("hex").slice(0, 12);
const tellTailSocialUrl = "/assets/tell-tail/ai-activity-social.png?v=" + assetVersion(new URL("design/assets/tell-tail/ai-activity-social.png", root));
const styleVersion = assetVersion(new URL("site/style.css", base));
const tellTailStyle = new URL("site/tell-tail/style.css", base);
const tellTailStyleUrl = "/assets/tell-tail/home.css?v=" + assetVersion(tellTailStyle);
const values = {
  tellTailHome,
  tellTailDocsUrl: tellTailHome + "docs/",
  tellTailVersion: e(tellTail.version),
  tellTailRepositoryUrl: tellTail.repositoryUrl,
  docsUrl,
  installUrl: docsOrigin + product.installPath,
  quickstartUrl: docsOrigin + product.quickstartPath,
  cloudUrl,
  cloudAuthUrl,
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
// The landing build owns these assets; the docs build owns dist/docs independently.
const tellTailAssets = new URL("assets/", tellTailOut);
rmSync(tellTailAssets, { recursive: true, force: true });
mkdirSync(tellTailAssets, { recursive: true });
for (const name of ["style.css", "site.js", "tokens.css", "mark.svg", "tell-tail"])
  cpSync(new URL("assets/" + name, out), new URL(name, tellTailAssets), { recursive: true });
cpSync(tellTailStyle, new URL("tell-tail/home.css", tellTailAssets));
cpSync(new URL("site/tell-tail/privacy.html", base), new URL("privacy.html", tellTailOut));
const scriptBytes = gzipSync(
  readFileSync(new URL("assets/site.js", out)),
).length;
if (scriptBytes > 50 * 1024)
  throw new Error(
    `Website JavaScript exceeds the 50 KiB gzip budget: ${scriptBytes} bytes`,
  );
for (const [filename, path, title, description] of pages) {
  const isTellTail = path === "tell-tail/";
  const pageDocsUrl = isTellTail ? "/docs/" : docsUrl;
  const pageValues = isTellTail ? { ...values, tellTailDocsUrl: pageDocsUrl } : values;
  const content = readFileSync(new URL(`site/${filename}`, base), "utf8").replace(
    /\{\{(\w+)\}\}/g,
    (_, key) => {
      if (!(key in pageValues)) throw new Error(`Unknown template value ${key}`);
      return pageValues[key];
    },
  );
  const canonical = isTellTail ? tellTail.websiteUrl + "/" : websiteUrl + "/" + path;
  const pageTitle = isTellTail ? e(title) : e(title) + " — Ding";
  const icon = isTellTail ? "/assets/tell-tail/logo.svg" : "/assets/mark.svg";
  const repositoryUrl = isTellTail ? tellTail.repositoryUrl : product.repositoryUrl;
  const accountActions = isTellTail
    ? `<a class="button" href="#install">Install Tell-Tail</a>`
    : `<a class="text-link" href="${cloudAuthUrl}">Sign in</a><a class="button primary" href="${cloudAuthUrl}">Create account</a>`;
  const footerLinks = isTellTail
    ? `<a href="${pageDocsUrl}">Tell-Tail docs</a><a href="/privacy">Privacy</a><a href="${repositoryUrl}">GitHub</a><a href="${repositoryUrl}/blob/${tellTail.releaseRef}/LICENSE">MIT license</a>`
    : `<a href="${cloudUrl}">Ding Cloud</a><a href="${cloudAuthUrl}">Sign in</a><a href="${cloudAuthUrl}">Create account</a><a href="${docsUrl}">Documentation</a><a href="${product.repositoryUrl}">GitHub</a><a href="${product.repositoryUrl}/blob/main/LICENSE">Apache-2.0</a>`;
  const headerBrand = isTellTail
    ? '<a class="wordmark tell-tail-wordmark" href="/" aria-label="Tell-Tail home"><img src="/assets/tell-tail/logo.svg" width="40" height="32" alt=""><span>tell-tail</span></a>'
    : '<a class="wordmark" href="/" aria-label="Ding home"><img src="/assets/mark.svg" width="34" height="34" alt=""><span>Ding<span class="wordmark-dot">.</span></span></a>';
  const footerBrand = isTellTail
    ? headerBrand
    : '<a class="wordmark" href="/"><img src="/assets/mark.svg" width="28" height="28" alt="">Ding.</a>';
  const footerCredit = isTellTail
    ? 'Created and maintained by <a href="https://github.com/zuchka">Matt Abrams</a>.'
    : 'Persistent watches. Durable alerts.';
  const navLinks = isTellTail ? [[pageDocsUrl, "Docs"], [repositoryUrl, "GitHub"]] : [
    ["/", "Ding"],
    [tellTailHome, "Tell-Tail"],
    [cloudUrl, "Cloud"],
    ["/console/", "Local Console"],
    ["/examples/", "Examples"],
    [pageDocsUrl, "Docs"],
    [repositoryUrl, "GitHub"],
  ];
  const nav = navLinks.map(
      ([href, label]) =>
        `<a href="${e(href)}"${href === "/" + path ? ' aria-current="page"' : ""}>${label}</a>`,
    )
    .join("");
  const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"><title>${pageTitle}</title><meta name="description" content="${e(description)}"><link rel="canonical" href="${canonical}"><meta property="og:title" content="${pageTitle}"><meta property="og:description" content="${e(description)}"><meta property="og:type" content="website"><meta property="og:url" content="${canonical}"><meta property="og:image" content="${isTellTail ? tellTail.websiteUrl + tellTailSocialUrl : websiteUrl + "/assets/social.png"}"><meta property="og:image:alt" content="${isTellTail ? "Tell-Tail AI Activity showing a synthetic Codex tool call that reported failure in 194 ms." : "Ding. Know what happened. See why."}"><meta name="twitter:card" content="summary_large_image">${preview || path === "404.html" || path === "console/" ? '<meta name="robots" content="noindex,follow">' : ""}<link rel="icon" href="${icon}" type="image/svg+xml"><link rel="stylesheet" href="/assets/tokens.css"><link rel="stylesheet" href="/assets/style.css?v=${styleVersion}">${isTellTail ? `<link rel="stylesheet" href="${tellTailStyleUrl}">` : ""}<script src="/assets/site.js" defer></script></head><body><a class="skip-link" href="#main">Skip to content</a><header class="site-header">${headerBrand}<nav aria-label="Main navigation">${nav}</nav><div class="account-actions">${accountActions}</div><button class="theme-toggle" type="button" hidden aria-label="Change color theme">Theme: system</button></header><main id="main">${content}</main><footer>${footerBrand}<p>${footerCredit}</p><div>${footerLinks}</div></footer></body></html>`;
  const dest = isTellTail ? new URL("index.html", tellTailOut) : new URL(filename, out);
  mkdirSync(new URL("./", dest), { recursive: true });
  writeFileSync(dest, html);
}
writeFileSync(
  new URL("sitemap.xml", out),
  `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${pages
    .filter((p) => !["404.html", "console/", "tell-tail/"].includes(p[1]))
    .map((p) => `<url><loc>${websiteUrl}/${p[1]}</loc></url>`)
    .join("")}</urlset>`,
);
writeFileSync(
  new URL("robots.txt", out),
  preview
    ? "User-agent: *\nDisallow: /\n"
    : `User-agent: *\nAllow: /\nDisallow: /console/\nSitemap: ${websiteUrl}/sitemap.xml\n`,
);
const landingCsp = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self' https://static.cloudflareinsights.com; object-src 'none'; frame-ancestors 'none'; base-uri 'none'";
writeFileSync(
  new URL("_headers", out),
  `/*\n  X-Content-Type-Options: nosniff\n  Referrer-Policy: strict-origin-when-cross-origin\n  Content-Security-Policy: ${landingCsp}\n/assets/*\n  Cache-Control: public, max-age=3600\n`,
);
writeFileSync(
  new URL("build-info.json", out),
  JSON.stringify(
    { source, runtime: product.runtime, console: product.console, tellTail, preview },
    null,
    2,
  ) + "\n",
);
writeFileSync(new URL("sitemap.xml", tellTailOut),
  `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>${tellTailHome}</loc></url><url><loc>${tellTailHome}privacy</loc></url></urlset>\n`);
writeFileSync(new URL("robots.txt", tellTailOut),
  `User-agent: *\nAllow: /\nDisallow: /docs/next/\nSitemap: ${tellTailHome}sitemap.xml\nSitemap: ${tellTailHome}docs/sitemap.xml\n`);
writeFileSync(new URL("_headers", tellTailOut),
  `/*\n  X-Content-Type-Options: nosniff\n  Referrer-Policy: strict-origin-when-cross-origin\n  X-Frame-Options: DENY\n/\n  Content-Security-Policy: ${landingCsp}\n/assets/*\n  Cache-Control: public, max-age=3600\n/docs/next/*\n  X-Robots-Tag: noindex, follow\n`);
writeFileSync(new URL("404.html", tellTailOut),
  '<!doctype html><html lang="en"><meta charset="utf-8"><title>Page not found · Tell-Tail</title><h1>Page not found</h1><p><a href="/">Tell-Tail home</a> · <a href="/docs/">Documentation</a></p></html>');
console.log(`Built website → ${fileURLToPath(out)}`);
