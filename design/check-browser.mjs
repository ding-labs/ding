import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const mark = readFileSync(
  new URL("./assets/mark.svg", import.meta.url),
  "utf8",
).trim();
const tellTailMark = readFileSync(
  new URL("./assets/tell-tail/logo.svg", import.meta.url),
  "utf8",
).trim();

// Run against built pages: this catches missing imports, stale copied assets and
// theme overrides that a source-only token check would miss.
export async function assertSharedDesign(page, surface) {
  const result = await page.evaluate((surface) => {
    const probe = document.createElement("span");
    probe.hidden = true;
    document.body.append(probe);
    const token = (name, property) => {
      probe.style.setProperty(property, `var(${name})`);
      return getComputedStyle(probe).getPropertyValue(property);
    };
    const errors = [];
    const check = (selector, property, name) => {
      for (const element of document.querySelectorAll(selector)) {
        const actual = getComputedStyle(element).getPropertyValue(property);
        const expected = token(name, property);
        if (actual !== expected)
          errors.push(
            `${selector}: ${property} ${actual}; expected ${expected}`,
          );
      }
    };
    check("body", "background-color", "--ding-bg");
    check("body", "color", "--ding-text");
    check("body", "font-family", "--ding-font");
    check(
      surface === "docs" ? ".md-typeset code" : "code",
      "font-family",
      "--ding-mono",
    );
    const action =
      surface === "docs" ? ".md-button--primary" : ".button.primary";
    check(action, "background-color", "--ding-action");
    check(action, "color", "--ding-on-brand");
    check(action, "border-radius", "--ding-radius-control");
    if (surface === "console") {
      check(".badge.good", "color", "--ding-success");
      check(".badge.unknown", "color", "--ding-unknown");
      check(".badge.warn", "color", "--ding-attention");
      check(".badge.neutral", "color", "--ding-paused");
    }
    const green = token("--ding-green", "color");
    probe.remove();
    const logoSelector =
      surface === "console"
        ? ".brand-bell"
        : surface === "docs"
          ? ".md-logo img"
          : ".wordmark img, .mini-brand img";
    return {
      errors,
      green,
      logos: [...document.querySelectorAll(logoSelector)].map((img) => img.src),
      tellTailLogo: document.querySelector("header .tell-tail-wordmark img")?.src,
      icon: document.querySelector('link[rel="icon"]')?.href,
    };
  }, surface);
  assert.deepEqual(
    result.errors,
    [],
    `Shared design mismatch at ${page.url()}`,
  );
  assert.equal(
    result.green,
    "rgb(126, 231, 135)",
    "Preserve the original #7EE787 green",
  );
  assert.ok(
    result.logos.length > 0,
    `${surface} must display its brand logo`,
  );
  if (surface === "tell-tail") {
    assert.ok(result.tellTailLogo, "Tell-Tail must have its own header logo");
    assert.equal(new URL(result.tellTailLogo).pathname, "/assets/tell-tail/logo.svg");
  }
  assert.ok(result.icon, `${surface} must have a bell favicon`);
  for (const url of new Set([...result.logos, result.icon])) {
    const response = await page.request.get(url);
    assert.ok(response.ok(), `Missing brand asset: ${url}`);
    assert.equal(
      (await response.text()).trim(),
      surface === "tell-tail" && url === result.tellTailLogo ? tellTailMark : mark,
      `Stale brand asset: ${url}`,
    );
  }
}
