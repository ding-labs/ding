import { test, expect } from "./fixtures";
import { call, manifest } from "./data";
import AxeBuilder from "@axe-core/playwright";
import { assertSharedDesign } from "../../../design/check-browser.mjs";

test("every Console section uses the shared design in both themes", async ({
  page,
  daemon,
}) => {
  test.setTimeout(90000);
  await call(daemon, "/apply", { manifest });
  for (let i = 0; i < 3; i++)
    await call(daemon, "/ingest/api-health", { status: 503 }, true);
  await page.goto(await daemon.launch());
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Appearance").selectOption(theme);
    for (const width of [375, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      for (const route of [
        "watches",
        "events",
        "deliveries",
        "workbench",
        "system",
      ]) {
        await page.goto(`${daemon.url}/ui/${route}`, {
          waitUntil: "networkidle",
        });
        await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
        await assertSharedDesign(page, "console");
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
        ).toBe(true);
        const result = await new AxeBuilder({ page }).analyze();
        expect(
          result.violations.filter((v) =>
            ["serious", "critical"].includes(v.impact || ""),
          ),
        ).toEqual([]);
      }
    }
  }
});
