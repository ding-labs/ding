import { readFileSync, existsSync } from "node:fs";
const root = new URL("../../", import.meta.url);
const dir = new URL("workers/docs/site/", root);
const channels = JSON.parse(
  readFileSync(new URL("channels.json", dir), "utf8"),
);
if (!channels.complete)
  throw new Error("Refusing to deploy current-only docs; build all channels");
const expected = JSON.parse(
  readFileSync(new URL("content/docs-versions.json", root), "utf8"),
).archives;
if (JSON.stringify(channels.archives) !== JSON.stringify(expected))
  throw new Error("Archive catalog drift; rebuild docs");
for (const path of [
  "index.html",
  "preview/index.html",
  "search/search_index.json",
  ...expected.map((a) => a.path + "/index.html"),
]) {
  if (!existsSync(new URL(path, dir)))
    throw new Error(`Missing published channel asset: ${path}`);
}
console.log("Complete documentation artifact verified");
