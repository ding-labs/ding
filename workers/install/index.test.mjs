import assert from "node:assert/strict";
import test from "node:test";
import worker from "./index.js";

test("browser visits redirect to the current install guide", async () => {
  const response = await worker.fetch(new Request("https://start.ding.ing/", {
    headers: { accept: "text/html,application/xhtml+xml" },
  }));

  assert.equal(response.status, 301);
  assert.equal(response.headers.get("location"), "https://docs.ding.ing/install/");
});

test("old curl installer requests fail without returning a shell script", async () => {
  const response = await worker.fetch(new Request("https://start.ding.ing/", {
    headers: { accept: "*/*" },
  }));

  assert.equal(response.status, 410);
  assert.equal(response.headers.get("content-type"), "text/plain; charset=utf-8");
  assert.match(await response.text(), /https:\/\/docs\.ding\.ing\/install\//);
});
