import { expect, type Page } from "@playwright/test";

export async function choosePreference(
  page: Page,
  name: string,
  option: string,
) {
  await page.getByRole("button", { name: "Open command menu" }).click();
  await page
    .getByRole("textbox", { name: "Search tasks and preferences" })
    .fill(name);
  await page.getByRole("button", { name: new RegExp(`^${name} `) }).click();
  await page.getByRole("button", { name: option, exact: true }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();
}

export async function setAppearance(page: Page, theme: string) {
  await choosePreference(
    page,
    "Appearance",
    theme[0].toUpperCase() + theme.slice(1),
  );
}
