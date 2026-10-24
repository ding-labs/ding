import AxeBuilder from "@axe-core/playwright";
import { test, expect } from "./fixtures";

test("settings preferences are accessible, responsive and persist", async ({
  page,
  daemon,
}, testInfo) => {
  await page.goto(await daemon.launch());
  await page.getByRole("button", { name: "Open account menu" }).click();
  await page.getByRole("menuitem", { name: "Settings", exact: true }).click();
  await expect(page).toHaveURL(/\/ui\/settings$/);
  await expect(
    page.getByRole("heading", { name: "Settings", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Display name", { exact: true })).toHaveCount(0);
  await page
    .getByRole("combobox", { name: "Appearance", exact: true })
    .selectOption("dark");
  await page
    .getByRole("combobox", { name: "Display density", exact: true })
    .selectOption("compact");
  await page
    .getByRole("combobox", { name: "Display time", exact: true })
    .selectOption("utc");
  await page.reload();
  await expect(
    page.getByRole("combobox", { name: "Appearance", exact: true }),
  ).toHaveValue("dark");
  await expect(
    page.getByRole("combobox", { name: "Display density", exact: true }),
  ).toHaveValue("compact");
  await expect(
    page.getByRole("combobox", { name: "Display time", exact: true }),
  ).toHaveValue("utc");
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`settings-${width}.png`),
      fullPage: true,
    });
  }
  await expect(page.locator(".settings-layout > .panel")).toHaveCSS(
    "background-color",
    "rgb(32, 39, 34)",
  );
  const a11y = await new AxeBuilder({ page }).include("main").analyze();
  expect(
    a11y.violations.filter((v) =>
      ["serious", "critical"].includes(v.impact || ""),
    ),
  ).toEqual([]);
});
