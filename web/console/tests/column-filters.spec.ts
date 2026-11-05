import AxeBuilder from "@axe-core/playwright";
import { test, expect } from "./fixtures";
import { call, manifest } from "./data";

test("watch column filters combine, persist, and stay editable with no matches", async ({
  page,
  daemon,
}, testInfo) => {
  for (const id of ["waiting", "open", "clear", "evaluated"]) {
    let definition = manifest
      .replaceAll("api-health", id)
      .replaceAll("API health", id);
    if (id === "evaluated")
      definition = definition.replace(
        "trigger: transition",
        "trigger: level, interval: 1m",
      );
    await call(daemon, "/apply", { manifest: definition });
    if (id !== "waiting") {
      for (let n = 0; n < 3; n++)
        await call(
          daemon,
          `/ingest/${id}`,
          { status: id === "open" ? 503 : 200 },
          true,
        );
    }
  }
  await call(daemon, "/watches/open/lifecycle", { action: "pause" });
  await page.goto(await daemon.launch());
  const table = page.getByRole("table");
  await expect(table.locator("tbody tr")).toHaveCount(4);
  for (const column of [
    "lifecycle",
    "condition",
    "source",
    "deliveries",
    "latest input",
  ])
    await expect(
      table.getByLabel(`Filter ${column}`, { exact: true }),
    ).toBeVisible();
  await page
    .getByLabel("Filter condition", { exact: true })
    .selectOption("open");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await expect(
    table.getByRole("link", { name: "open", exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Filter lifecycle", { exact: true })
    .selectOption("paused");
  await page
    .getByLabel("Filter source", { exact: true })
    .selectOption("received");
  await page
    .getByLabel("Filter latest input", { exact: true })
    .selectOption("day");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  page.once("dialog", (dialog) => dialog.accept("Open paused watches"));
  await page.getByRole("button", { name: "Save current view" }).click();
  await page.reload();
  await expect(
    page.getByLabel("Filter condition", { exact: true }),
  ).toHaveValue("open");
  await expect(
    page.getByLabel("Filter lifecycle", { exact: true }),
  ).toHaveValue("paused");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await page
    .getByLabel("Filter deliveries", { exact: true })
    .selectOption("failed");
  await expect(
    page.getByRole("heading", { name: "No watches match this view" }),
  ).toBeVisible();
  await expect(
    table.getByLabel("Filter condition", { exact: true }),
  ).toBeVisible();
  await page.getByLabel("Filter deliveries", { exact: true }).selectOption("");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await page
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(table.locator("tbody tr")).toHaveCount(4);
  await page
    .getByLabel("Saved view", { exact: true })
    .selectOption("Open paused watches");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await expect(
    page.getByLabel("Filter condition", { exact: true }),
  ).toHaveValue("open");
  await page
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(table.locator("tbody tr")).toHaveCount(4);
  for (const [value, id] of [
    ["waiting", "waiting"],
    ["clear", "clear"],
    ["evaluated", "evaluated"],
  ]) {
    await page
      .getByLabel("Filter condition", { exact: true })
      .selectOption(value);
    await expect(table.locator("tbody tr")).toHaveCount(1);
    await expect(
      table.getByRole("link", { name: id, exact: true }),
    ).toBeVisible();
  }
  await page
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(table.locator("tbody tr")).toHaveCount(4);
  await page
    .getByLabel("Filter latest input", { exact: true })
    .selectOption("never");
  await expect(
    table.getByRole("link", { name: "waiting", exact: true }),
  ).toBeVisible();
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await page
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(table.locator("tbody tr")).toHaveCount(4);
  const accessibility = await new AxeBuilder({ page })
    .include("table")
    .analyze();
  expect(accessibility.violations).toEqual([]);
  await page.setViewportSize({ width: 375, height: 900 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth + 1,
    ),
  ).toBe(true);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.screenshot({
    path: testInfo.outputPath("column-filters.png"),
    fullPage: true,
  });
});

test("changing a column filter resets pagination", async ({ page, daemon }) => {
  for (const id of ["a-watch", "b-watch", "c-watch"])
    await call(daemon, "/apply", {
      manifest: manifest
        .replaceAll("api-health", id)
        .replaceAll("API health", id),
    });
  await page.goto(await daemon.launch());
  await expect(
    page.getByRole("link", { name: "a-watch", exact: true }),
  ).toBeVisible();
  await page.goto(`${daemon.url}/ui/watches?limit=1`);
  await expect(
    page.getByRole("link", { name: "a-watch", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(
    page.getByRole("link", { name: "b-watch", exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Filter condition", { exact: true })
    .selectOption("waiting");
  await expect(
    page.getByRole("link", { name: "a-watch", exact: true }),
  ).toBeVisible();
  expect(new URL(page.url()).searchParams.has("cursor")).toBe(false);
  await expect(
    page.getByText("3 matching records", { exact: true }),
  ).toBeVisible();
});
