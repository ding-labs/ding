import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
const dist = new URL("../dist/", import.meta.url);
const tellTailDist = new URL("../../tell-tail/dist/", import.meta.url);
const html = readFileSync(new URL("index.html", tellTailDist), "utf8");
const build = JSON.parse(readFileSync(new URL("build-info.json", dist)));
const homeUrl = (build.preview ? build.tellTail.previewWebsiteUrl : build.tellTail.websiteUrl) + "/";
const docs = homeUrl + "docs/";
test("Tell-Tail has its own install flow, repository, and license", () => {
  assert.match(html, /href="#install">Install Tell-Tail/);
  assert.doesNotMatch(html, /Create account|>Apache-2.0</);
  assert.ok(html.includes('href="/docs/"'));
  assert.ok(html.includes('Created and maintained by <a href="https://github.com/zuchka">Matt Abrams</a>'));
  assert.doesNotMatch(html, /Ding Labs|href="https:\/\/ding\.ing/);
  assert.ok(html.includes(`href="${build.tellTail.repositoryUrl}/blob/${build.tellTail.releaseRef}/LICENSE"`));
  assert.match(html, /brew install ding-labs\/tap\/tell-tail/);
  assert.ok(html.includes(`cargo install tell-tail --version ${build.tellTail.version} --locked`));
  assert.match(html, /AI Activity · Synthetic tool result/);
  assert.doesNotMatch(html, /\{\{\w+\}\}/);
});
test("the optional Codex catalog follows both standalone install options", () => {
  const sectionStart = html.indexOf('<section id="use-in-codex"');
  const cargo = html.indexOf("cargo install tell-tail");
  assert.ok(html.indexOf("brew install ding-labs/tap/tell-tail") < cargo && cargo < sectionStart);
  const section = html.slice(sectionStart, html.indexOf("</section>", sectionStart));
  const button = section.match(/<a class="button primary" href="([^"]+)">Use in Codex<\/a>/);
  assert.ok(button, "Use in Codex is a link styled as a button");
  assert.equal(button[1], "codex://plugins/install/tell-tail?marketplace=tell-tail-plugins");
  assert.ok(section.includes("Apple Silicon"));
  assert.ok(section.includes("Add a marketplace"));
  assert.ok(section.includes('<code id="catalog-source">zuchka/tell-tail-plugins</code>'));
  assert.ok(section.includes('id="copy-catalog" hidden>Copy catalog</button>'));
  assert.ok(section.includes('href="https://github.com/zuchka/tell-tail-plugins"'));
});
test("Tell-Tail is discoverable and has product-specific share metadata", () => {
  assert.ok(readFileSync(new URL("sitemap.xml", tellTailDist), "utf8").includes("https://tell-tail.dev/"));
  assert.match(html, /rel="canonical" href="https:\/\/tell-tail\.dev\/"/);
  assert.match(html, /<title>Tell-Tail — Local AI observability/);
  const image = readFileSync(new URL("assets/tell-tail/ai-activity-social.png", tellTailDist));
  assert.deepEqual([image.readUInt32BE(16), image.readUInt32BE(20)], [1200, 630]);
  const version = createHash("sha256").update(image).digest("hex").slice(0, 12);
  assert.ok(html.includes(`property="og:image" content="https://tell-tail.dev/assets/tell-tail/ai-activity-social.png?v=${version}"`));
  assert.match(html, /property="og:image:alt" content="Tell-Tail AI Activity/);
});

test("the installed AI Activity and MCP demos have distinct entry points", () => {
  assert.match(html, /id="hero-title".*id="activity".*id="mcp".*id="install".*id="logs-title"/s);
  assert.match(html, /href="#activity">Explore AI Activity/);
  assert.match(html, /href="#mcp">Try the MCP demo/);
  assert.match(html, /tell-tail desktop --demo --open/);
  assert.match(html, /tell-tail mcp --demo --open/);
  assert.ok(html.includes('href="/docs/plugin-tracing/#local-call-inspection"'));
});

test("the responsive homepage screenshots ship at their declared dimensions", () => {
  for (const file of ["tool-result", "activity", "activity-mobile", "mcp-recovery", "log-throughput"]) {
    const path = `/assets/tell-tail/${file}.png`;
    const image = readFileSync(new URL("." + path, tellTailDist));
    const dimensions = `width="${image.readUInt32BE(16)}" height="${image.readUInt32BE(20)}"`;
    assert.ok(html.includes(`="${path}" ${dimensions}`), path);
  }
});

test("the homepage features Tell-Tail before the detailed Ding sections", () => {
  const home = readFileSync(new URL("index.html", dist), "utf8");
  const feature = home.indexOf('class="tell-tail-feature"');
  assert.ok(home.includes("Tell-Tail by Matt Abrams"));
  assert.ok(feature > 0 && feature < home.indexOf('id="cloud"'));
  assert.ok(home.includes(`href="${homeUrl}">Explore Tell-Tail`));
  assert.ok(home.includes(`href="${docs}"`));
});

test("former landing page and docs URLs keep their destinations and query parameters", async () => {
  const website = (await import("../src/worker.js")).default;
  const guides = (await import("../../docs/src/worker.js")).default;
  const moved = [
    [website, "www.ding.ing/tell-tail", "/"],
    [website, "ding.ing/tell-tail/index.html", "/"],
    [website, "ding.ing/assets/tell-tail/dashboard.png", "/assets/tell-tail/dashboard.png"],
    [guides, "docs.ding.ing/tell-tail", "/docs/"],
    [guides, "docs.ding.ing/tell-tail/mcp/", "/docs/mcp/"],
    [guides, "docs.ding.ing/tell-tail/next/mcp/index.html", "/docs/next/mcp/"],
    [guides, "docs.ding.ing/tell-tail/next/search/search_index.json", "/docs/next/search/search_index.json"],
  ];
  for (const [worker, old, path] of moved) {
    const response = await worker.fetch(new Request("https://" + old + "?from=old"), {});
    assert.equal(response.status, 301);
    assert.equal(response.headers.get("location"), "https://tell-tail.dev" + path + "?from=old");
  }
  const assets = { fetch: () => new Response("unrelated", { status: 404 }) };
  assert.equal((await guides.fetch(new Request("https://docs.ding.ing/tell-tailored"), { ASSETS: assets })).status, 404);
});
