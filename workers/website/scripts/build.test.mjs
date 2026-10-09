import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, existsSync } from "node:fs";
import { gzipSync } from "node:zlib";
import worker from "../src/worker.js";
import { validateProduct } from "../../../scripts/web/product.mjs";
const root = new URL("../../../", import.meta.url);
const product = JSON.parse(readFileSync(new URL("content/product.json", root)));
const dist = new URL("../dist/", import.meta.url);

test("canonical host redirects preserve path and query", async () => {
  const r = await worker.fetch(
    new Request("https://www.ding.ing/examples/?ref=docs"),
    {},
  );
  assert.equal(r.status, 301);
  assert.equal(
    r.headers.get("location"),
    "https://ding.ing/examples/?ref=docs",
  );
});
test("legacy recipes point to their exact frozen docs", async () => {
  const r = await worker.fetch(
    new Request("https://ding.ing/recipes/mlflow/?from=old"),
    {},
  );
  assert.equal(r.status, 301);
  assert.equal(
    r.headers.get("location"),
    "https://docs.ding.ing/legacy/v0.14.0/recipes/mlflow/?from=old",
  );
  const removed = await worker.fetch(
    new Request("https://ding.ing/recipes/circleci"),
    {},
  );
  assert.equal(
    removed.headers.get("location"),
    "https://docs.ding.ing/legacy/#retired-recipes",
  );
});
test("missing assets keep their 404 instead of becoming an application page", async () => {
  const request = new Request("https://ding.ing/no-such-file.js");
  const r = await worker.fetch(request, {
    ASSETS: {
      fetch: async (req) => {
        assert.equal(req, request);
        return new Response("Missing", { status: 404 });
      },
    },
  });
  assert.equal(r.status, 404);
});
test("availability cannot advertise a console in a legacy or preview binary", () => {
  assert.doesNotThrow(() => validateProduct(product));
  const p = structuredClone(product);
  p.console = { status: "available", minimumVersion: "v1.0.0" };
  assert.throws(() => validateProduct(p), /stable/);
  p.runtime = { channel: "stable", version: "v0.15.0", sourceRef: "v0.15.0" };
  assert.throws(() => validateProduct(p), /predates/);
  p.runtime.version = "v1.0.0";
  assert.doesNotThrow(() => validateProduct(p));
});
test("built pages have metadata, truthful preview labels, and no unresolved templates", () => {
  for (const file of [
    "index.html",
    "console/index.html",
    "examples/index.html",
    "404.html",
  ]) {
    const html = readFileSync(new URL(file, dist), "utf8");
    assert.match(html, /<html lang="en">/);
    assert.match(html, /rel="canonical"/);
    assert.match(html, /name="description"/);
    assert.doesNotMatch(html, /\{\{(?:docsUrl|snippet:|evidence|manifest)/);
    assert.doesNotMatch(
      html,
      /MIT licensed|No database|ding run --|116k|5MB static binary/,
    );
  }
  assert.match(
    readFileSync(new URL("console/index.html", dist), "utf8"),
    /not a released interface/,
  );
  assert.ok(existsSync(new URL("assets/social.png", dist)));
  assert.ok(
    gzipSync(readFileSync(new URL("assets/site.js", dist))).length < 50 * 1024,
  );
});

test('docs review deployments are excluded from indexing without changing production', async () => {
  const {default: docs} = await import('../../docs/src/worker.js');
  const request = new Request('https://ding-docs-preview.workers.dev/');
  const assets = {fetch: async () => new Response('docs', {headers: {'Content-Type': 'text/html'}})};
  const preview = await docs.fetch(request, {ASSETS: assets, PREVIEW: 'true'});
  assert.equal(preview.headers.get('X-Robots-Tag'), 'noindex, follow');
  const production = await docs.fetch(request, {ASSETS: assets});
  assert.equal(production.headers.has('X-Robots-Tag'), false);
  const robots = await docs.fetch(new Request('https://ding-docs-preview.workers.dev/robots.txt'), {PREVIEW:'true'});
  assert.match(await robots.text(), /Disallow: \//);
});
