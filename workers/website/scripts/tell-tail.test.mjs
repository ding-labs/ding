import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
const dist = new URL("../dist/", import.meta.url);
const html = readFileSync(new URL("tell-tail/index.html", dist), "utf8");
const info = JSON.parse(readFileSync(new URL("build-info.json", dist)));
const product = JSON.parse(readFileSync(new URL("../../../content/product.json", import.meta.url)));
const docs = (info.preview ? product.previewDocsUrl : product.docsUrl) + "/tell-tail/";
test("Tell-Tail has its own install flow, repository, and license", () => {
  assert.match(html, /href="#install">Install Tell-Tail/);
  assert.doesNotMatch(html, /Create account|>Apache-2.0</);
  assert.ok(html.includes(`href="${docs}"`));
  assert.match(html, /github.com\/ding-labs\/tell-tail\/blob\/v0.2.0\/LICENSE/);
  assert.match(html, /brew install ding-labs\/tap\/tell-tail/);
  assert.match(html, /cargo install tell-tail --version 0.2.0 --locked/);
  assert.match(html, /synthetic sample logs/);
  assert.doesNotMatch(html, /\{\{\w+\}\}/);
});
test("Tell-Tail is discoverable and has product-specific share metadata", () => {
  assert.ok(readFileSync(new URL("sitemap.xml", dist), "utf8").includes("https://ding.ing/tell-tail/"));
  assert.match(html, /rel="canonical" href="https:\/\/ding.ing\/tell-tail\/"/);
  assert.match(html, /property="og:image" content="https:\/\/ding.ing\/assets\/tell-tail\/dashboard.png"/);
  const image = readFileSync(new URL("assets/tell-tail/dashboard.png", dist));
  assert.ok(html.includes(`width="${image.readUInt32BE(16)}" height="${image.readUInt32BE(20)}"`));
});
