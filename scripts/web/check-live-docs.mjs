import { readFileSync } from "node:fs";
import { loadProduct } from "./product.mjs";
const product = loadProduct(new URL("../../", import.meta.url));
for (const path of [
  product.installPath,
  product.quickstartPath,
  "/legacy/v0.14.0/",
  "/tell-tail/",
  "/tell-tail/dashboard/",
  "/tell-tail/next/",
  "/tell-tail/next/mcp/",
]) {
  const response = await fetch(product.docsUrl + path, {
    signal: AbortSignal.timeout(15000),
  });
  if (
    !response.ok ||
    !response.headers.get("content-type")?.includes("text/html")
  )
    throw new Error(
      `Docs are not ready at ${product.docsUrl + path}; deploy and verify docs before website publication`,
    );
}
const info = await fetch(product.docsUrl + "/build-info.json", {
  signal: AbortSignal.timeout(15000),
});
if (
  !info.ok ||
  (await info.json()).runtime?.sourceRef !== product.runtime.sourceRef
)
  throw new Error("Published docs do not match the advertised runtime");
const tellTail = JSON.parse(readFileSync(new URL("../../content/tell-tail.json", import.meta.url), "utf8"));
for (const [path, ref] of [["/tell-tail/", tellTail.docsRef], ["/tell-tail/next/", tellTail.previewRef]]) {
  const response = await fetch(product.docsUrl + path + "build-info.json", { signal: AbortSignal.timeout(15000) });
  if (!response.ok) throw new Error("Publish Tell-Tail docs before the website");
  const info = await response.json();
  if (info.source !== ref || info.version !== tellTail.version)
    throw new Error("Published Tell-Tail docs do not match the website; publish Docs first");
}
console.log("Production docs are ready for website links");
