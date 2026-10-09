import { test, expect } from "./fixtures";
import { call, manifest } from "./data";
import AxeBuilder from "@axe-core/playwright";
test("responsive themes, accessibility and keyboard navigation", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  for (let i = 0; i < 3; i++)
    await call(daemon, "/ingest/api-health", { status: 503 }, true);
  await page.goto(await daemon.launch());
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Appearance").selectOption(theme);
    for (const width of [375, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(daemon.url + "/ui/watches/api-health");
      await expect(
        page.getByRole("heading", { name: "Incident opened", exact: true }),
      ).toBeVisible();
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth + 1,
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
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.keyboard.press(
    process.platform === "darwin" ? "Meta+k" : "Control+k",
  );
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByLabel("Search navigation commands").fill("Doctor");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("heading", { name: "Doctor", exact: true }),
  ).toBeVisible();
  const a = await new AxeBuilder({ page }).analyze();
  expect(
    a.violations.filter((v) =>
      ["serious", "critical"].includes(v.impact || ""),
    ),
  ).toEqual([]);
  await page.getByRole("link", { name: "Workbench", exact: true }).click();
  await page.getByRole("button", { name: /Receive a signal/ }).click();
  await expect(
    page.getByText("Valid definition", { exact: false }),
  ).toBeVisible();
  await page.getByLabel("Syntax editor", { exact: true }).check();
  await expect(
    page.getByRole("textbox", { name: "Manifest syntax editor" }),
  ).toContainText("apiVersion");
  const w = await new AxeBuilder({ page }).analyze();
  expect(
    w.violations.filter((v) =>
      ["serious", "critical"].includes(v.impact || ""),
    ),
  ).toEqual([]);
});
test("draft survives logout and reconnect in the same page session", async ({
  page,
  daemon,
  context,
}) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Workbench", exact: true }).click();
  await page.getByLabel("Manifest", { exact: true }).fill(manifest);
  await page.getByRole("button", { name: "Log out" }).click();
  await expect(
    page.getByRole("heading", { name: "Open your console." }),
  ).toBeVisible();
  const other = await context.newPage();
  await other.goto(await daemon.launch());
  await expect(
    other.getByRole("navigation", { name: "Main navigation" }),
  ).toBeVisible();
  await other.close();
  await page.getByRole("button", { name: "Reconnect", exact: true }).click();
  await expect(page.getByLabel("Manifest", { exact: true })).toHaveValue(
    manifest,
  );
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(
    "consecutive",
  );
});
