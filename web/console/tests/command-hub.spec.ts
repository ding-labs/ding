import { test, expect } from "./fixtures";
import { choosePreference, setAppearance } from "./preferences";
import AxeBuilder from "@axe-core/playwright";

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
  await expect(search).toBeFocused();
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

test("account menu opens preferences with focus and returns to normal command search", async ({
  page,
  daemon,
}, testInfo) => {
  await page.goto(await daemon.launch());
  await page.getByRole("button", { name: "Open account menu" }).click();
  await expect(page.getByRole("menu")).toContainText("Local owner");
  await page.getByRole("menuitem", { name: "Preferences" }).click();
  const search = page.getByRole("textbox", {
    name: "Search tasks and preferences",
  });
  await expect(search).toBeFocused();
  await expect(search).toHaveValue("Preferences");
  await expect(page.locator(".command-results button")).toHaveCount(3);
  const accessibility = await new AxeBuilder({ page })
    .include('[role="dialog"]')
    .analyze();
  expect(
    accessibility.violations.filter((v) =>
      ["serious", "critical"].includes(v.impact || ""),
    ),
  ).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("preferences.png") });
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Open command menu" }).click();
  await expect(search).toHaveValue("");
  await expect(
    page.getByRole("button", { name: "Inspect events" }),
  ).toBeVisible();
});

test("workspace switching keeps preferences separate and fits long names", async ({
  page,
  daemon,
}, testInfo) => {
  const workspaceName = "Personal workspace for development and operations";
  await page.route("**/v1/browser/session", async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.data = {
      ...body.data,
      execution: "shared",
      workspace: route.request().headers()["x-ding-workspace"] || "personal",
      user: { id: "test-owner", name: "Ding user" },
      role: "owner",
      capabilities: ["manage", "preview"],
      memberships: [
        { workspace: "personal", name: workspaceName },
        { workspace: "team", name: "Team workspace" },
      ],
    };
    await route.fulfill({ response, json: body });
  });
  await page.goto(await daemon.launch());
  await choosePreference(page, "Display density", "Compact");
  await page.getByRole("combobox", { name: "Workspace" }).selectOption("team");
  await expect(page.getByRole("combobox", { name: "Workspace" })).toHaveValue(
    "team",
  );
  await expect(page.locator("html")).toHaveAttribute(
    "data-density",
    "comfortable",
  );
  await page
    .getByRole("combobox", { name: "Workspace" })
    .selectOption("personal");
  await expect(page.locator("html")).toHaveAttribute("data-density", "compact");
  for (const width of [1440, 768, 375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    const header = page.locator(".location-bar");
    expect((await header.boundingBox())!.height).toBeLessThan(80);
    await header.screenshot({
      path: testInfo.outputPath(`header-${width}.png`),
    });
  }
  await page.getByRole("button", { name: "Open account menu" }).click();
  await expect(page.getByRole("menu")).toContainText("Ding user");
  await expect(page.getByRole("menu")).toContainText(
    `owner · ${workspaceName}`,
  );
});
