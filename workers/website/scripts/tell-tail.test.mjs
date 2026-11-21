import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
const dist = new URL("../dist/", import.meta.url);
const tellTailDist = new URL("../../tell-tail/dist/", import.meta.url);
const html = readFileSync(new URL("index.html", tellTailDist), "utf8");
const docs = "https://tell-tail.dev/docs/";
test("Tell-Tail has its own install flow, repository, and license", () => {
  assert.match(html, /href="#install">Install Tell-Tail/);
  assert.doesNotMatch(html, /Create account|>Apache-2.0</);
  assert.ok(html.includes(`href="${docs}"`));
  assert.match(html, /github.com\/ding-labs\/tell-tail\/blob\/v0.4.0\/LICENSE/);
  assert.match(html, /brew install ding-labs\/tap\/tell-tail/);
  assert.match(html, /cargo install tell-tail --version 0.4.0 --locked/);
  assert.match(html, /synthetic sample logs/);
  assert.doesNotMatch(html, /\{\{\w+\}\}/);
});
test("Tell-Tail is discoverable and has product-specific share metadata", () => {
  assert.ok(readFileSync(new URL("sitemap.xml", tellTailDist), "utf8").includes("https://tell-tail.dev/"));
  assert.match(html, /rel="canonical" href="https:\/\/tell-tail\.dev\/"/);
  assert.match(html, /property="og:image" content="https:\/\/tell-tail\.dev\/assets\/tell-tail\/dashboard.png\?v=[a-f0-9]{12}"/);
  const image = readFileSync(new URL("assets/tell-tail/dashboard.png", tellTailDist));
  assert.ok(html.includes(`width="${image.readUInt32BE(16)}" height="${image.readUInt32BE(20)}"`));
});

test("the homepage features Tell-Tail before the detailed Ding sections", () => {
  const home = readFileSync(new URL("index.html", dist), "utf8");
  const feature = home.indexOf('class="tell-tail-feature"');
  assert.ok(feature > 0 && feature < home.indexOf('id="cloud"'));
  assert.match(home, /href="https:\/\/tell-tail\.dev\/">Explore Tell-Tail/);
  assert.ok(home.includes(`href="${docs}"`));
});
