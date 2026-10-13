import { test, expect } from "./fixtures";
import { choosePreference, setAppearance } from "./preferences";

test("command preferences persist and remain available on a narrow screen", async ({
  page,
  daemon,
}) => {
  await page.goto(await daemon.launch());
  await page.setViewportSize({ width: 375, height: 800 });
  await choosePreference(page, "Display density", "Compact");
  await choosePreference(page, "Display time", "UTC");
  await setAppearance(page, "dark");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.locator("html")).toHaveAttribute("data-density", "compact");
  await page.getByRole("button", { name: "Open command menu" }).click();
  await expect(
    page.getByRole("button", { name: "Display time UTC" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Appearance Dark" }).click();
  await expect(
    page.getByRole("button", { name: "Dark", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("heading", { name: "Go to a task or preference" }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Search tasks and preferences" }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("button", { name: "Open command menu" }),
  ).toBeFocused();
});

test("keyboard search handles settings, empty results and navigation", async ({
  page,
  daemon,
}) => {
  await page.goto(await daemon.launch());
  await expect(
    page.getByRole("button", { name: "Open command menu" }),
  ).toBeVisible();
  await page.keyboard.press(
    process.platform === "darwin" ? "Meta+k" : "Control+k",
  );
  const search = page.getByRole("textbox", {
    name: "Search tasks and preferences",
  });
  await search.fill("no such command");
  await expect(page.getByRole("status")).toHaveText(
    "No matching tasks or preferences.",
  );
  await page.keyboard.press("ArrowDown");
  await expect(search).toBeFocused();
  await search.fill("density");
  await page.keyboard.press("Enter");
  await page.keyboard.press("ArrowUp");
  await expect(
    page.getByRole("button", { name: "Compact", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.locator("html")).toHaveAttribute("data-density", "compact");
  await search.fill("Inspect events");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/ui\/events$/);
  await expect(page.getByRole("dialog")).toBeHidden();
});
