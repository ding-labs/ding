import { test, expect } from "./fixtures";
import { call, manifest } from "./data";
import { mkdirSync } from "node:fs";
import { resolve } from "node:path";
test("capture the isolated evidence-first fixture for visual review", async ({
  page,
  daemon,
}) => {
  test.skip(
    process.env.DING_VISUAL_CAPTURE !== "1",
    "Opt-in documentation screenshots; always use an isolated fixture.",
  );
  const dir = resolve(import.meta.dirname, "../../../docs/images/console");
  mkdirSync(dir, { recursive: true });
  for (const [id, name, n] of [
    ["api-health", "Preview · Payments API", 3],
    ["inventory", "Preview · Inventory updates", 1],
    ["worker", "Preview · Background worker", 0],
  ] as const) {
    await call(daemon, "/apply", {
      manifest: manifest
        .replaceAll("api-health", id)
        .replace("name: API health", `name: ${name}`),
    });
    for (let i = 0; i < n; i++)
      await call(daemon, `/ingest/${id}`, { status: 503 }, true);
  }
  await call(daemon, "/watches/worker/lifecycle", { action: "pause" });
  await page.goto(await daemon.launch());
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByLabel("Appearance").selectOption("light");
  await expect(
    page.getByRole("link", { name: "Preview · Payments API" }),
  ).toBeVisible();
  await page.screenshot({ path: resolve(dir, "watches-light.png") });
  await page.getByRole("link", { name: "Preview · Payments API" }).click();
  await expect(
    page.getByRole("heading", { name: "Incident opened", exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: resolve(dir, "evidence-light.png") });
  await page.getByLabel("Appearance").selectOption("dark");
  await page.screenshot({ path: resolve(dir, "evidence-dark.png") });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.screenshot({ path: resolve(dir, "evidence-mobile.png") });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByLabel("Appearance").selectOption("light");
  await page.getByRole("link", { name: "Workbench", exact: true }).click();
  await page.getByRole("button", { name: /Receive a signal/ }).click();
  await expect(
    page.getByText("Valid definition", { exact: false }),
  ).toBeVisible();
  await page.screenshot({ path: resolve(dir, "workbench-light.png") });
});
