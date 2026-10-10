import { test, expect } from "@playwright/test";
import { build } from "esbuild";
import { readFile } from "node:fs/promises";

test("official MCP Apps bridge renders, adapts, and navigates offline", async ({
  page,
}) => {
  const html = await readFile(
    new URL(
      "../../../integrations/mcp/src/ding_mcp/assets/workspace.html",
      import.meta.url,
    ),
    "utf8",
  );
  const built = await build({
    entryPoints: ["tests/host.ts"],
    bundle: true,
    write: false,
    format: "esm",
    platform: "browser",
    target: "es2022",
  });
  const host = built.outputFiles[0].text;
  const external: string[] = [];
  await page.route("**/*", async (route) => {
    const u = new URL(route.request().url());
    if (u.origin !== "https://ding-test.local") {
      external.push(u.href);
      return route.abort();
    }
    if (u.pathname === "/widget")
      return route.fulfill({
        contentType: "text/html",
        headers: {
          "Content-Security-Policy":
            "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'none'; base-uri 'none'",
        },
        body: html,
      });
    if (u.pathname === "/host.js")
      return route.fulfill({ contentType: "text/javascript", body: host });
    return route.fulfill({
      contentType: "text/html",
      body: '<!doctype html><style>body{margin:0;background:#f4f4f2}iframe{border:0;width:100%;height:850px}</style><iframe title="Ding" src="/widget"></iframe><script type="module" src="/host.js"></script>',
    });
  });
  await page.goto("https://ding-test.local/?theme=light");
  const frame = page.frameLocator("iframe");
  await expect(frame.getByText("Payments API", { exact: true })).toBeVisible();
  await expect(
    frame.getByRole("heading", { name: "A little peace of mind." }),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/ding-mcp-light.png",
    fullPage: true,
  });
  await frame.getByRole("button", { name: "Events", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(frame.getByText("No events in this view.")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 800 });
  await page.goto("https://ding-test.local/?theme=dark");
  await expect(frame.getByText("Payments API", { exact: true })).toBeVisible();
  await page.screenshot({
    path: "test-results/ding-mcp-dark-mobile.png",
    fullPage: true,
  });
  const width = await frame
    .locator("body")
    .evaluate((el) => ({ scroll: el.scrollWidth, client: el.clientWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
  expect(external).toEqual([]);
});
