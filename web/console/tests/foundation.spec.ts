import { test, expect } from "./fixtures";
test("one-use launch, deep link, appearance and logout", async ({
  page,
  daemon,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const link = await daemon.launch();
  await page.goto(link);
  await expect(
    page.getByRole("navigation", { name: "Main navigation" }),
  ).toBeVisible();
  await expect(page).not.toHaveURL(/handoff/);
  expect(await page.evaluate(() => localStorage.getItem("admin"))).toBeNull();
  await page.getByLabel("Appearance").selectOption("light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.goto(daemon.url + "/ui/events");
  await expect(
    page.getByRole("heading", {
      name: "The story, as it happened.",
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Log out" }).click();
  await expect(
    page.getByRole("heading", { name: "Open your console." }),
  ).toBeVisible();
  await page.goto(link);
  await expect(page.getByRole("alert")).toContainText("expired");
  expect(errors).toEqual([]);
});
