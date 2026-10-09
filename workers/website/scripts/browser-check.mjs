import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { spawn } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { createServer } from "node:net";
import assert from "node:assert/strict";
const root = new URL("../../../", import.meta.url);
const artifacts = new URL("artifacts/public-surfaces/", root);
await mkdir(artifacts, { recursive: true });
const children = [];
async function freePort() {
  const s = createServer();
  await new Promise((r) => s.listen(0, "127.0.0.1", r));
  const p = s.address().port;
  await new Promise((r) => s.close(r));
  return p;
}
async function serve(surface, existing) {
  if (existing) return existing;
  const port = await freePort();
  const cwd = new URL(`workers/${surface}/`, root);
  const child = spawn(
    process.execPath,
    [
      "node_modules/wrangler/bin/wrangler.js",
      "dev",
      "--local",
      "--ip",
      "127.0.0.1",
      "--port",
      String(port),
      "--inspector-port",
      "0",
    ],
    {
      cwd,
      detached: process.platform !== "win32",
      env: { ...process.env, WRANGLER_SEND_METRICS: "false" },
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
  children.push(child);
  let logs = "";
  child.stdout.on("data", (d) => (logs += d));
  child.stderr.on("data", (d) => (logs += d));
  const url = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 120; i++) {
    if (child.exitCode !== null)
      throw new Error(`${surface} preview exited: ${logs}`);
    try {
      if ((await fetch(url)).ok) return url;
    } catch {}
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`${surface} preview did not start: ${logs}`);
}
let browser;
const report = { pages: [], errors: [], checks: [], performance: [] };
try {
  const website = await serve("website", process.env.SITE_URL);
  const docs = await serve("docs", process.env.DOCS_URL);
  browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE }
      : {}),
  });
  const routes = [
    ["home", website + "/"],
    ["console", website + "/console/"],
    ["examples", website + "/examples/"],
    ["docs", docs + "/"],
    ["quickstart", docs + "/guides/first-watch/"],
    ["reference", docs + "/reference/manifest/"],
    ["console-help", docs + "/console/"],
  ];
  for (const theme of ["light", "dark"])
    for (const width of [375, 768, 1024, 1440]) {
      const context = await browser.newContext({
        viewport: { width, height: 1000 },
        colorScheme: theme,
        reducedMotion: "reduce",
      });
      for (const [name, url] of routes) {
        const page = await context.newPage();
        page.on("pageerror", (error) =>
          report.errors.push(`${name}: ${error.message}`),
        );
        const response = await page.goto(url, { waitUntil: "networkidle" });
        assert.equal(response.status(), 200);
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth + 1,
        );
        if (overflow)
          report.errors.push(
            `${name}/${theme}/${width}: horizontal page overflow`,
          );
        if (width === 375 || width === 1440) {
          const accessibility = await new AxeBuilder({ page }).analyze();
          for (const violation of accessibility.violations.filter((v) =>
            ["serious", "critical"].includes(v.impact),
          ))
            report.errors.push(
              `${name}/${theme}/${width}: ${violation.id} ${violation.nodes.map((n) => n.target.join(" ")).join(", ")}`,
            );
        }
        if (width === 375 || width === 1440)
          await page.screenshot({
            path: fileURLToPath(
              new URL(`${name}-${theme}-${width}.png`, artifacts),
            ),
            fullPage: true,
          });
        report.pages.push({ name, theme, width, status: response.status() });
        if (name === "home" && theme === "light" && width === 375)
          report.performance = await page.evaluate(() =>
            performance
              .getEntriesByType("resource")
              .map((r) => ({
                name: new URL(r.name).pathname,
                transferSize: r.transferSize,
                duration: r.duration,
              })),
          );
        await page.close();
      }
      await context.close();
    }
  const page = await browser.newPage({
    viewport: { width: 1024, height: 900 },
  });
  await page.goto(website + "/");
  await page.getByRole("button", { name: "Recovery", exact: true }).click();
  await page
    .getByRole("button", { name: "Recovery", exact: true })
    .evaluate((el) => {
      if (el.getAttribute("aria-pressed") !== "true")
        throw new Error("Recovery control failed");
    });
  assert.equal(
    await page.locator("[data-demo-event]").textContent(),
    "Recovered",
  );
  await page.getByRole("button", { name: "Firing", exact: true }).click();
  assert.equal(await page.locator("[data-demo-event]").textContent(), "Firing");
  const theme = page.getByRole("button", { name: /Color theme:/ });
  await theme.focus();
  await page.keyboard.press("Enter");
  assert.equal(await page.locator("html").getAttribute("data-theme"), "light");
  await page.reload();
  assert.equal(await page.locator("html").getAttribute("data-theme"), "light");
  await page.keyboard.press("Tab");
  // A 1024px physical display at 200% zoom exposes 512 CSS pixels.
  // CSS body.zoom does not update media queries and is not browser zoom.
  const zoomContext = await browser.newContext({viewport:{width:512,height:450},deviceScaleFactor:2});
  const zoomPage = await zoomContext.newPage();
  for (const url of [website + "/", docs + "/guides/first-watch/"]) {
    await zoomPage.goto(url);
    assert.equal(await zoomPage.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, `200% zoom reflow: ${url}`);
  }
  await zoomContext.close();
  const noJS = await browser.newContext({
    javaScriptEnabled: false,
    viewport: { width: 375, height: 900 },
  });
  const staticPage = await noJS.newPage();
  await staticPage.goto(website + "/");
  assert.ok(await staticPage.getByRole("heading", { level: 1 }).isVisible());
  assert.ok(
    await staticPage
      .getByRole("link", { name: /Try the watch preview/ })
      .isVisible(),
  );
  await noJS.close();
  for (const base of [website, docs]) {
    assert.equal((await fetch(base + "/missing-page")).status, 404);
    assert.equal((await fetch(base + "/missing-asset.js")).status, 404);
  }
  assert.equal(
    (await fetch(website + "/console", { redirect: "manual" })).status,
    307,
  );
  const legacy = await fetch(website + "/recipes/mlflow", {
    redirect: "manual",
  });
  assert.equal(legacy.status, 301);
  assert.match(
    legacy.headers.get("location"),
    /legacy\/v0\.14\.0\/recipes\/mlflow\//,
  );
  await page.emulateMedia({colorScheme: 'dark'});
  await page.goto(docs + "/");
  await page.getByTitle('Switch to light theme', {exact:true}).click();
  assert.equal(await page.locator('body').getAttribute('data-md-color-scheme'), 'default');
  const lightBackground = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  await page.reload();
  assert.equal(await page.locator('body').getAttribute('data-md-color-scheme'), 'default');
  await page.getByTitle('Switch to dark theme', {exact:true}).click();
  assert.notEqual(await page.evaluate(() => getComputedStyle(document.body).backgroundColor), lightBackground);
  await page
    .getByRole("textbox", { name: "Search", exact: true })
    .fill("idempotency");
  await page.waitForFunction(() =>
    document
      .querySelector(".md-search-result__meta")
      ?.textContent?.includes("matching"),
  );
  assert.ok((await page.locator(".md-search-result__link").count()) > 0);
  report.checks = [
    "fixture controls",
    "keyboard theme control and persistence",
    "200% zoom",
    "no-JavaScript content",
    "Worker 404s",
    "trailing slash",
    "legacy redirect",
    "docs search",
    "docs manual theme overrides system preference and persists",
  ];
  await writeFile(
    new URL("report.json", artifacts),
    JSON.stringify(report, null, 2),
  );
  assert.deepEqual(report.errors, []);
  console.log(
    `Passed ${report.pages.length} page/theme/viewport checks; accessibility, search, routing, and interactions. Screenshots: ${fileURLToPath(artifacts)}`,
  );
} finally {
  await writeFile(new URL('report.json', artifacts), JSON.stringify(report, null, 2));
  await browser?.close();
  for (const child of children) {
    try {
      process.platform === "win32"
        ? child.kill()
        : process.kill(-child.pid, "SIGTERM");
    } catch {}
  }
}
