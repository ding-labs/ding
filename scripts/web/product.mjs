import { readFileSync } from "node:fs";

export function validateProduct(p) {
  for (const key of [
    "name",
    "description",
    "license",
    "websiteUrl",
    "docsUrl",
    "previewDocsUrl",
    "repositoryUrl",
  ]) {
    if (typeof p[key] !== "string" || !p[key])
      throw new Error(`Missing product ${key}`);
  }
  for (const key of ["websiteUrl", "docsUrl", "previewDocsUrl", "repositoryUrl"]) {
    if (new URL(p[key]).protocol !== "https:")
      throw new Error(`${key} must use HTTPS`);
  }
  if (!["source-preview", "stable"].includes(p.runtime?.channel))
    throw new Error("Unknown runtime channel");
  if (
    !/^[a-f0-9]{7,40}$|^v\d+\.\d+\.\d+(?:[-.][\w.-]+)?$/.test(
      p.runtime.sourceRef,
    )
  )
    throw new Error("Pin a runtime source ref");
  if (
    p.runtime.channel === "stable" &&
    !/^v\d+\.\d+\.\d+$/.test(p.runtime.version || "")
  )
    throw new Error("Stable runtime needs a version");
  if (p.runtime.channel === "source-preview" && p.runtime.version !== null)
    throw new Error("Source preview cannot claim a release");
  if (!["design-preview", "available"].includes(p.console?.status))
    throw new Error("Unknown console status");
  if (p.console.status === "available") {
    if (
      p.runtime.channel !== "stable" ||
      !/^v\d+\.\d+\.\d+$/.test(p.console.minimumVersion || "")
    )
      throw new Error("Console requires a stable minimum version");
    const current = p.runtime.version.slice(1).split(".").map(Number);
    const minimum = p.console.minimumVersion.slice(1).split(".").map(Number);
    if (
      current.some(
        (v, i) =>
          v < minimum[i] &&
          current.slice(0, i).every((n, j) => n === minimum[j]),
      )
    )
      throw new Error("Runtime predates console");
  } else if (p.console.minimumVersion !== null)
    throw new Error("Preview console cannot claim a minimum release");
  for (const key of ["installPath", "quickstartPath"])
    if (!/^\/[\w/-]+\/$/.test(p[key])) throw new Error(`Invalid ${key}`);
  return p;
}

export function loadProduct(root) {
  return validateProduct(
    JSON.parse(readFileSync(new URL("content/product.json", root), "utf8")),
  );
}
export const escapeHtml = (value) =>
  String(value).replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
