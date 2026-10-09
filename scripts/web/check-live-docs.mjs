import { loadProduct } from "./product.mjs";
const product = loadProduct(new URL("../../", import.meta.url));
for (const path of [
  product.installPath,
  product.quickstartPath,
  "/legacy/v0.14.0/",
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
console.log("Production docs are ready for website links");
